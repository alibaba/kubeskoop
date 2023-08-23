package bpfutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	log "k8s.io/klog/v2"
)

func removeBTFinfo(path string) error {
	// llvm-objcopy -R .BTF.ext [obj_path]
	output, err := exec.Command("llvm-objcopy", "-R", ".BTF.ext", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error remove btf info from object: %v, output %v, err: %v", path, string(output), err)
	}
	return nil
}

var standardCFlags = []string{"-O2", "-target", "bpf", "-std=gnu89"}

func CompileBPF(name string) ([]byte, error) {

	outputPath := filepath.Join("/var/lib/bpf/", fmt.Sprintf("%s.o", name))
	args := make([]string, 0, 16)
	args = append(args, "-g")
	args = append(args, standardCFlags...)
	args = append(args, "-I/var/lib/bpf/headers")
	args = append(args, "-c")
	args = append(args, filepath.Join("/var/lib/bpf/", fmt.Sprintf("%s.c", name)))
	args = append(args, "-o")
	args = append(args, outputPath)
	switch runtime.GOARCH {
	case "amd64":
		args = append(args, "-D__TARGET_ARCH_x86")
	case "arm64":
		args = append(args, "-D__TARGET_ARCH_arm64")
	default:
		return nil, fmt.Errorf("unsupport arch")
	}

	cmd := exec.Command("clang", args...)
	log.Info("exec", "cmd", cmd.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			log.Info(string(out))
		}
		return nil, err
	}
	if len(out) > 0 {
		log.Info(string(out))
	}
	defer func() {
		os.Remove(outputPath)
	}()
	err = removeBTFinfo(outputPath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(outputPath)
}
