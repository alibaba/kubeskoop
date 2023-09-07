package nettop

import (
	fmt "fmt"
	"os"
	"strings"
	"unsafe"

	log "github.com/sirupsen/logrus"

	"golang.org/x/sys/unix"
)

//go:generate protoc --go_out=. ./libnettop.proto

var (
	top              = metacache{}
	runtimeEndpoints = []string{"/var/run/dockershim.sock", "/run/containerd/containerd.sock", "/run/k3s/containerd/containerd.sock"}

	sidecarEnabled bool
)

type metacache struct {
	NodeMeta
}

func Init(sidecar bool) {
	top.NodeName = getNodeName()
	kr, err := getKernelRelease()
	if err != nil {
		log.Errorf("failed to get node kernel info %v", err)
	} else {
		top.Kernel = kr
	}
	if !sidecar {
		// use empty cri meta, fulfillize it when updated
		c := &CriMeta{}
		err = c.Update()
		if err != nil {
			log.Errorf("update cri meta failed %v", err)
		}

		top.Crimeta = c
	}

	sidecarEnabled = sidecar
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
