package bpfutil

import (
	"runtime"

	"github.com/cilium/ebpf"
)

func UpdateFeatureSwitch(m *ebpf.Map, key int, value uint8) error {
	numCPUs := runtime.NumCPU()
	fsEnableFlowPortValues := make([]uint8, numCPUs)
	for i := 0; i < numCPUs; i++ {
		fsEnableFlowPortValues[i] = value
	}
	return m.Update(uint32(key), fsEnableFlowPortValues, ebpf.UpdateAny)
}
