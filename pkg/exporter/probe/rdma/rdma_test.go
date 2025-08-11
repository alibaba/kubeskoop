package rdma

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vishvananda/netlink"
)

const (
	testDeviceNameMlx5  = "mlx5_0"
	testDeviceNameErdma = "erdma_0"
)

// MockNetlink for simulating netlink operations
type MockNetlink struct {
	mock.Mock
}

func (m *MockNetlink) RdmaResourceList() ([]*netlink.RdmaResource, error) {
	args := m.Called()
	return args.Get(0).([]*netlink.RdmaResource), args.Error(1)
}

func (m *MockNetlink) RdmaLinkByName(name string) (*netlink.RdmaLink, error) {
	args := m.Called(name)
	return args.Get(0).(*netlink.RdmaLink), args.Error(1)
}

func (m *MockNetlink) RdmaStatistic(link *netlink.RdmaLink) (*netlink.RdmaDeviceStatistic, error) {
	args := m.Called(link)
	return args.Get(0).(*netlink.RdmaDeviceStatistic), args.Error(1)
}

func TestMetricsFromSysFS(t *testing.T) {
	// Create temporary directory structure to simulate /sys/class/infiniband
	tmpDir := t.TempDir()
	devName := testDeviceNameMlx5
	portName := "1"

	// Create device directory and port directory
	devPath := filepath.Join(tmpDir, devName)
	portPath := filepath.Join(devPath, "ports", portName)
	countersPath := filepath.Join(portPath, "counters")

	// Create directory structure
	assert.NoError(t, os.MkdirAll(countersPath, 0755))

	// Create test counter files
	counterFiles := map[string]string{
		"port_rcv_packets":  "100",
		"port_xmit_packets": "200",
	}

	for name, value := range counterFiles {
		filePath := filepath.Join(countersPath, name)
		assert.NoError(t, os.WriteFile(filePath, []byte(value), 0644))
	}

	// Create test RdmaLink
	link := &netlink.RdmaLink{
		Attrs: netlink.RdmaLinkAttrs{
			Name: devName,
		},
	}

	// Call the tested function
	stats, err := metricsFromSysFS(link, tmpDir)

	// Verify results
	assert.NoError(t, err)
	assert.Len(t, stats, 1)
	assert.Equal(t, uint32(1), stats[0].PortIndex)
	assert.Equal(t, uint64(100), stats[0].Statistics["port_rcv_packets"])
	assert.Equal(t, uint64(200), stats[0].Statistics["port_xmit_packets"])
}

func TestRdmaLinkType(t *testing.T) {
	tests := []struct {
		name     string
		linkName string
		expected string
	}{
		{"Mellanox device", testDeviceNameMlx5, linkTypeMellanox},
		{"ERdma device", testDeviceNameErdma, linkTypeERdma},
		{"Unknown device", "unknown_device", linkTypeUnknown},
		{"Empty name", "", linkTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := &netlink.RdmaLink{
				Attrs: netlink.RdmaLinkAttrs{Name: tt.linkName},
			}
			assert.Equal(t, tt.expected, rdmaLinkType(link))
		})
	}

	// Test nil link
	assert.Equal(t, linkTypeUnknown, rdmaLinkType(nil))
}

func TestCollectOnce(t *testing.T) {
	// Create temporary directory structure to simulate /sys/class/infiniband
	tmpDir := t.TempDir()
	devName := testDeviceNameMlx5
	portName := "1"

	// Create device directory and port directory
	devPath := filepath.Join(tmpDir, devName)
	portPath := filepath.Join(devPath, "ports", portName)
	countersPath := filepath.Join(portPath, "counters")

	// Create directory structure
	assert.NoError(t, os.MkdirAll(countersPath, 0755))

	// Create test counter files
	counterFiles := map[string]string{
		"port_rcv_packets":  "100",
		"port_xmit_packets": "200",
	}

	for name, value := range counterFiles {
		filePath := filepath.Join(countersPath, name)
		assert.NoError(t, os.WriteFile(filePath, []byte(value), 0644))
	}

	// Create mock netlink object
	mockNetlink := new(MockNetlink)

	// Set up mock behavior
	mockNetlink.On("RdmaResourceList").Return([]*netlink.RdmaResource{
		{Name: devName, RdmaResourceSummaryEntries: map[string]uint64{"qp": 10}},
	}, nil)

	mockNetlink.On("RdmaLinkByName", devName).Return(&netlink.RdmaLink{
		Attrs: netlink.RdmaLinkAttrs{Name: devName},
	}, nil)

	mockNetlink.On("RdmaStatistic", mock.Anything).Return(&netlink.RdmaDeviceStatistic{
		RdmaPortStatistics: []*netlink.RdmaPortStatistic{
			{
				PortIndex: 1,
				Statistics: map[string]uint64{
					"port_rcv_data":  50,
					"port_xmit_data": 60,
				},
			},
		},
	}, nil)

	// Create test metricsProbe
	p := &metricsProbe{
		netlink:  mockNetlink,
		basePath: tmpDir, // Use temporary directory as base path
	}

	// Create mock emit function
	emitted := make(map[string]float64)
	emit := func(metric string, _ []string, value float64) {
		emitted[metric] = value
	}

	// Execute collectOnce
	err := p.collectOnce(emit)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, float64(10), emitted["qp"])                      // Resource summary metric
	assert.Equal(t, float64(50), emitted["mlx5_port_rcv_data"])      // Netlink statistic metric
	assert.Equal(t, float64(60), emitted["mlx5_port_xmit_data"])     // Netlink statistic metric
	assert.Equal(t, float64(100), emitted["mlx5_port_rcv_packets"])  // Sysfs counter metric
	assert.Equal(t, float64(200), emitted["mlx5_port_xmit_packets"]) // Sysfs counter metric
	mockNetlink.AssertExpectations(t)
}

func TestCollectOnceNoCounters(t *testing.T) {
	devName := testDeviceNameErdma

	// Create mock netlink object
	mockNetlink := new(MockNetlink)

	// Set up mock behavior
	mockNetlink.On("RdmaResourceList").Return([]*netlink.RdmaResource{
		{Name: devName, RdmaResourceSummaryEntries: map[string]uint64{"qp": 10}},
	}, nil)

	mockNetlink.On("RdmaLinkByName", devName).Return(&netlink.RdmaLink{
		Attrs: netlink.RdmaLinkAttrs{Name: devName},
	}, nil)

	mockNetlink.On("RdmaStatistic", mock.Anything).Return(&netlink.RdmaDeviceStatistic{
		RdmaPortStatistics: []*netlink.RdmaPortStatistic{
			{
				PortIndex: 1,
				Statistics: map[string]uint64{
					"port_rcv_data":  50,
					"port_xmit_data": 60,
				},
			},
		},
	}, nil)

	// Create test metricsProbe
	p := &metricsProbe{
		netlink: mockNetlink,
	}

	// Create mock emit function
	emitted := make(map[string]float64)
	emit := func(metric string, _ []string, value float64) {
		emitted[metric] = value
	}

	// Execute collectOnce
	err := p.collectOnce(emit)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, float64(10), emitted["qp"])                   // Resource summary metric
	assert.Equal(t, float64(50), emitted["erdma_port_rcv_data"])  // Netlink statistic metric
	assert.Equal(t, float64(60), emitted["erdma_port_xmit_data"]) // Netlink statistic metric
	mockNetlink.AssertExpectations(t)
}

func TestMetricsProbeCreator(t *testing.T) {
	// Call the tested function
	probe, err := metricsProbeCreator()

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, probe)
	assert.Equal(t, "rdma", probe.Name())

	err = probe.Start(context.Background())
	assert.NoError(t, err)
	err = probe.Stop(context.Background())
	assert.NoError(t, err)
}

func TestMetricsFromSysFSErrors(t *testing.T) {
	// Test invalid link
	_, err := metricsFromSysFS(nil, "")
	assert.Error(t, err)

	// Test non-existent directory
	link := &netlink.RdmaLink{
		Attrs: netlink.RdmaLinkAttrs{Name: "invalid_device"},
	}
	_, err = metricsFromSysFS(link, "/nonexistent/path")
	assert.Error(t, err)

	// Test invalid port directory
	tmpDir := t.TempDir()
	devName := testDeviceNameMlx5
	devPath := filepath.Join(tmpDir, devName)
	assert.NoError(t, os.MkdirAll(devPath, 0755))

	// Create invalid port directory (not a number)
	invalidPortPath := filepath.Join(devPath, "ports", "invalid_port")
	assert.NoError(t, os.MkdirAll(invalidPortPath, 0755))

	link = &netlink.RdmaLink{
		Attrs: netlink.RdmaLinkAttrs{Name: devName},
	}
	stats, err := metricsFromSysFS(link, tmpDir)
	assert.NoError(t, err) // Function skips invalid ports
	assert.Len(t, stats, 0)

	// Test invalid counter file
	portName := "1"
	portPath := filepath.Join(devPath, "ports", portName)
	countersPath := filepath.Join(portPath, "counters")
	assert.NoError(t, os.MkdirAll(countersPath, 0755))

	// Create invalid counter file (non-numeric content)
	invalidCounterPath := filepath.Join(countersPath, "invalid_counter")
	assert.NoError(t, os.WriteFile(invalidCounterPath, []byte("not a number"), 0644))

	stats, err = metricsFromSysFS(link, tmpDir)
	assert.NoError(t, err)
	assert.Len(t, stats, 1)
	assert.Len(t, stats[0].Statistics, 0) // Invalid counter is skipped
}

func TestCollectOnceErrors(t *testing.T) {
	// Create mock netlink object
	mockNetlink := new(MockNetlink)

	// Set up mock behavior: return error
	mockNetlink.On("RdmaResourceList").Return([]*netlink.RdmaResource{}, fmt.Errorf("mock error"))

	// Create test metricsProbe
	p := &metricsProbe{
		netlink: mockNetlink,
	}

	// Create mock emit function
	emit := func(_ string, _ []string, _ float64) {}

	// Execute collectOnce, expected to return error
	err := p.collectOnce(emit)
	assert.Error(t, err)
	mockNetlink.AssertExpectations(t)

	// Reset mock, set partial operations successful partial failure
	mockNetlink = new(MockNetlink)
	mockNetlink.On("RdmaResourceList").Return([]*netlink.RdmaResource{
		{Name: testDeviceNameMlx5, RdmaResourceSummaryEntries: map[string]uint64{"qp": 10}},
		{Name: "invalid_device"},
	}, nil)

	mockNetlink.On("RdmaLinkByName", testDeviceNameMlx5).Return(&netlink.RdmaLink{
		Attrs: netlink.RdmaLinkAttrs{Name: testDeviceNameMlx5},
	}, nil)

	mockNetlink.On("RdmaLinkByName", "invalid_device").Return(&netlink.RdmaLink{}, fmt.Errorf("device not found"))

	mockNetlink.On("RdmaStatistic", mock.Anything).Return(&netlink.RdmaDeviceStatistic{}, fmt.Errorf("statistics error"))

	p = &metricsProbe{
		netlink: mockNetlink,
	}

	// Execute collectOnce, expected partial success
	err = p.collectOnce(emit)
	assert.NoError(t, err) // Function internally handles errors, does not return
	mockNetlink.AssertExpectations(t)
}
