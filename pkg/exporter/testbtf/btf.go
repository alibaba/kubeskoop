package testbtf

import (
	"log"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

const mapKey uint32 = 0

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang bpf ../../bpf/kprobe.c -- -I../../bpf/headers

func btfTest(btf *btf.Spec) error {
	fn := "sys_execve"

	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Printf("set mem limit:%s", err)
		return err
	}

	opts := ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			KernelTypes: btf,
		},
	}

	objs := bpfObjects{}
	if err := loadBpfObjects(&objs, &opts); err != nil {
		log.Printf("loading objects: %v", err)
		return err
	}
	defer objs.Close()

	kp, err := link.Kprobe(fn, objs.KprobeExecve, nil)
	if err != nil {
		log.Printf("opening kprobe: %s", err)
		return err
	}
	defer kp.Close()

	var value uint64
	if err := objs.KprobeMap.Lookup(mapKey, &value); err != nil {
		log.Printf("reading map: %v", err)
		return err
	}
	log.Printf("%s called %d times\n", fn, value)
	return nil
}

func RunBTFTest(btf *btf.Spec) error {
	return btfTest(btf)
}
