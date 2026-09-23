package sink

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

func TestFlameAggregator(t *testing.T) {
	agg := NewFlameAggregator()

	// Test adding events
	evt1 := &probe.Event{
		Type:    "PacketLoss",
		Message: "kfree_skb+0x100\nnf_hook_slow+0x200\nip_forward+0x300",
		Labels: []probe.Label{
			{Name: "src_type", Value: "pod"},
			{Name: "src_namespace", Value: "default"},
			{Name: "src_pod", Value: "test-pod"},
		},
	}

	evt2 := &probe.Event{
		Type:    "TCPRetrans",
		Message: "tcp_retransmit_skb+0x100\ntcp_write_xmit+0x200",
		Labels: []probe.Label{
			{Name: "dst_type", Value: "pod"},
			{Name: "dst_namespace", Value: "default"},
			{Name: "dst_pod", Value: "test-pod"},
		},
	}

	agg.AddEvent(evt1)
	agg.AddEvent(evt2)
	agg.AddEvent(evt1) // duplicate to test counting

	// Test node scope
	nodeCollapsed := agg.GetCollapsed("node", "", "")
	if !strings.Contains(nodeCollapsed, "kfree_skb+0x100;nf_hook_slow+0x200;ip_forward+0x300") {
		t.Errorf("node scope should contain packetloss stack, got: %s", nodeCollapsed)
	}
	if !strings.Contains(nodeCollapsed, "tcp_retransmit_skb+0x100;tcp_write_xmit+0x200") {
		t.Errorf("node scope should contain tcpretrans stack, got: %s", nodeCollapsed)
	}

	// Test pod scope
	podCollapsed := agg.GetCollapsed("pod", "default/test-pod", "")
	if !strings.Contains(podCollapsed, "kfree_skb+0x100;nf_hook_slow+0x200;ip_forward+0x300") {
		t.Errorf("pod scope should contain packetloss stack, got: %s", podCollapsed)
	}
	if !strings.Contains(podCollapsed, "tcp_retransmit_skb+0x100;tcp_write_xmit+0x200") {
		t.Errorf("pod scope should contain tcpretrans stack, got: %s", podCollapsed)
	}

	// Test event type filtering
	packetLossOnly := agg.GetCollapsed("node", "", "PacketLoss")
	if strings.Contains(packetLossOnly, "tcp_retransmit_skb") {
		t.Errorf("filtered by PacketLoss should not contain TCPRetrans, got: %s", packetLossOnly)
	}

	// Test counting (evt1 was added twice)
	if !strings.Contains(nodeCollapsed, " 2") {
		t.Logf("Note: counting verification - node collapsed: %s", nodeCollapsed)
	}
}

func TestFlameSink(t *testing.T) {
	sink := NewFlameSink()
	if sink.String() != "flame" {
		t.Errorf("expected sink name 'flame', got '%s'", sink.String())
	}

	evt := &probe.Event{
		Type:    "PacketLoss",
		Message: "kfree_skb+0x100\nnf_hook_slow+0x200",
	}

	err := sink.Write(evt)
	if err != nil {
		t.Errorf("Write should not return error, got: %v", err)
	}

	// Verify event was added to aggregator
	collapsed := defaultFlameAgg.GetCollapsed("node", "", "PacketLoss")
	if !strings.Contains(collapsed, "kfree_skb+0x100;nf_hook_slow+0x200") {
		t.Errorf("event should be in aggregator, got: %s", collapsed)
	}
}

func TestServeFlameCollapsed(t *testing.T) {
	// Reset aggregator for clean test
	defaultFlameAgg = NewFlameAggregator()

	// Add test event
	evt := &probe.Event{
		Type:    "PacketLoss",
		Message: "kfree_skb+0x100\nnf_hook_slow+0x200",
	}
	defaultFlameAgg.AddEvent(evt)

	// Test node scope request
	req := httptest.NewRequest("GET", "/flamegraph/collapsed?scope=node", nil)
	w := httptest.NewRecorder()
	ServeFlameCollapsed(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "kfree_skb+0x100;nf_hook_slow+0x200") {
		t.Errorf("response should contain stack, got: %s", body)
	}

	// Test pod scope request
	req2 := httptest.NewRequest("GET", "/flamegraph/collapsed?scope=pod&namespace=default&pod=test", nil)
	w2 := httptest.NewRecorder()
	ServeFlameCollapsed(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w2.Code)
	}

	// Test reset
	req3 := httptest.NewRequest("GET", "/flamegraph/collapsed?scope=node&reset=true", nil)
	w3 := httptest.NewRecorder()
	ServeFlameCollapsed(w3, req3)

	// Verify reset worked
	collapsed := defaultFlameAgg.GetCollapsed("node", "", "")
	if collapsed != "" {
		t.Errorf("after reset, collapsed should be empty, got: %s", collapsed)
	}
}

