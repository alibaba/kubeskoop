package cmd

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type blockingCollector struct {
	started chan struct{}
	release chan struct{}
	first   atomic.Bool
	desc    *prometheus.Desc
}

func (c *blockingCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }
func (c *blockingCollector) Collect(ch chan<- prometheus.Metric) {
	if c.first.CompareAndSwap(false, true) {
		close(c.started)
		<-c.release
	}
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, 1)
}

func TestMetricsServerAllowsConcurrentScrapesAfterCancellationAndReload(t *testing.T) {
	collector := &blockingCollector{started: make(chan struct{}), release: make(chan struct{}), desc: prometheus.NewDesc("test_collection", "Test collection", nil, nil)}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(collector.release) }) }
	t.Cleanup(release)
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	server := &MetricsServer{prometheusRegistry: registry}
	server.SetDisableCompression(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/metrics", nil).WithContext(ctx))
	}()
	select {
	case <-collector.started:
	case <-time.After(5 * time.Second):
		t.Fatal("collection did not start")
	}
	cancel()
	// A canceled, still-running collection and a handler reload must not block
	// other collectors. Launch all requests before waiting for their responses.
	server.SetDisableCompression(true)
	const clients = 20
	responses := make(chan int, clients)
	for i := 0; i < clients; i++ {
		go func() {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			responses <- response.Code
		}()
	}
	for i := 0; i < clients; i++ {
		select {
		case code := <-responses:
			if code != http.StatusOK {
				t.Fatalf("concurrent scrape status %d, want 200", code)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent scrape blocked behind the canceled collection")
		}
	}

	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("collection did not finish")
	}
	if first.Code != http.StatusOK {
		t.Fatalf("first scrape status %d", first.Code)
	}
	next := httptest.NewRecorder()
	server.ServeHTTP(next, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if next.Code != http.StatusOK {
		t.Fatalf("next scrape status %d, want 200", next.Code)
	}
}

// pipeListener gives the real HTTP server an unbuffered connection, so a
// non-reading client deterministically blocks response writes.
type pipeListener struct {
	conn     net.Conn
	closed   chan struct{}
	once     sync.Once
	accepted bool
}

func (l *pipeListener) Accept() (net.Conn, error) {
	if !l.accepted {
		l.accepted = true
		return l.conn, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}
func (l *pipeListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *pipeListener) Addr() net.Addr { return l.conn.LocalAddr() }

type observedWriteConn struct {
	net.Conn
	started chan struct{}
	once    sync.Once
}

func (c *observedWriteConn) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Write(p)
}

func serveMetricsOnPipe(t *testing.T, server *MetricsServer) (written, returned <-chan struct{}) {
	t.Helper()
	client, conn := net.Pipe()
	observed := &observedWriteConn{Conn: conn, started: make(chan struct{})}
	done := make(chan struct{})
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		server.ServeHTTP(w, r)
	}))
	if err := ts.Listener.Close(); err != nil {
		t.Fatal(err)
	}
	ts.Listener = &pipeListener{conn: observed, closed: make(chan struct{})}
	ts.Start()
	t.Cleanup(ts.Close)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client connection: %v", err)
		}
	})
	if err := client.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write([]byte("GET /metrics HTTP/1.1\r\nHost: test\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	return observed.started, done
}

func TestMetricsServerBoundsBlockedWritesWithoutBlockingOtherScrapes(t *testing.T) {
	registry := prometheus.NewRegistry()
	// Exceed net/http's response buffer so ServeHTTP itself blocks writing.
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "large_response", Help: "test", ConstLabels: prometheus.Labels{"padding": strings.Repeat("x", 64*1024)}})
	registry.MustRegister(gauge)
	server := &MetricsServer{prometheusRegistry: registry, writeTimeout: time.Second}
	server.SetDisableCompression(true)
	written, returned := serveMetricsOnPipe(t, server)
	select {
	case <-written:
	case <-time.After(5 * time.Second):
		t.Fatal("response write did not start")
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("concurrent scrape during blocked write: got %d, want 200", response.Code)
	}
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("write deadline did not release blocked response")
	}
	next := httptest.NewRecorder()
	server.ServeHTTP(next, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if next.Code != http.StatusOK {
		t.Fatalf("after write timeout: got %d, want 200", next.Code)
	}
}

func TestMetricsServerWriteDeadlineDoesNotBlockOtherCollectors(t *testing.T) {
	collector := &blockingCollector{started: make(chan struct{}), release: make(chan struct{}), desc: prometheus.NewDesc("slow_collection", "test", nil, nil)}
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	server := &MetricsServer{prometheusRegistry: registry, writeTimeout: 50 * time.Millisecond}
	server.SetDisableCompression(true)
	_, returned := serveMetricsOnPipe(t, server)
	var once sync.Once
	release := func() { once.Do(func() { close(collector.release) }) }
	t.Cleanup(release)
	select {
	case <-collector.started:
	case <-time.After(5 * time.Second):
		t.Fatal("collection did not start")
	}
	// The actual connection deadline expires while Gather remains blocked.
	time.Sleep(100 * time.Millisecond)
	select {
	case <-returned:
		t.Fatal("handler returned before collection completed")
	default:
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("concurrent scrape after another request deadline: got %d, want 200", response.Code)
	}
	release()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not finish after collection")
	}
	next := httptest.NewRecorder()
	server.ServeHTTP(next, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if next.Code != http.StatusOK {
		t.Fatalf("after collection: got %d, want 200", next.Code)
	}
}
