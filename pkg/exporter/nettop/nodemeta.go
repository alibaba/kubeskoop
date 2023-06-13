package nettop

import (
	fmt "fmt"
	"io"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

//go:generate protoc --go_out=. ./libnettop.proto

var (
	logger *slog.Logger

	top              = metacache{}
	runtimeEndpoints = []string{"/var/run/dockershim.sock", "/run/containerd/containerd.sock"}
)

type metacache struct {
	NodeMeta
}

func init() {
	logger = slog.New(slog.NewJSONHandler(io.Discard))
	top.NodeName = getNodeName()
	kr, err := getKernelRelease()
	if err != nil {
		logger.Warn("failed to get node kernel info %s", err.Error())
	} else {
		top.Kernel = kr
	}
	// use empty cri meta, fulfillize it when updated
	c := &CriMeta{}
	err = c.Update()
	if err != nil {
		logger.Warn("update cri meta failed %s", err.Error())
	}

	top.Crimeta = c
}

func getNodeName() string {
	if os.Getenv("INSPECTOR_NODENAME") != "" {
		return os.Getenv("INSPECTOR_NODENAME")
	}
	node, err := os.Hostname()
	if err != nil {
		return "Unknow"
	}

	return node
}

func getKernelRelease() (*Kernel, error) {
	k := &Kernel{}
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return k, fmt.Errorf("uname failed: %w", err)
	}
	k.Version = unix.ByteSliceToString(uname.Version[:])
	k.Release = unix.ByteSliceToString(uname.Release[:])
	k.Architecture = strings.TrimRight(string((*[65]byte)(unsafe.Pointer(&uname.Machine))[:]), "\000")
	return k, nil
}

func GetRuntimeName() string {
	return top.NodeMeta.GetCrimeta().GetRuntimeName()
}

func GetRuntimeVersion() string {
	return top.NodeMeta.GetCrimeta().GetRuntimeVersion()
}

func GetRuntimeSock() string {
	return top.NodeMeta.GetCrimeta().GetRuntimeSock()
}

func GetRuntimeAPIVersion() string {
	return top.NodeMeta.GetCrimeta().GetVersion()
}

func GetNodeName() string {
	return top.GetNodeName()
}

func GetKernelRelease() string {
	return top.Kernel.Release
}
