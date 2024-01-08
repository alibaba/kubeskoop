package nettop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	log "github.com/sirupsen/logrus"

	"github.com/patrickmn/go-cache"
	"github.com/vishvananda/netns"
)

var (
	cacheUpdateInterval = 10 * time.Second
	nsCache             = cache.New(20*cacheUpdateInterval, 20*cacheUpdateInterval)
	pidCache            = cache.New(20*cacheUpdateInterval, 20*cacheUpdateInterval)
	ipCache             = cache.New(20*cacheUpdateInterval, 20*cacheUpdateInterval)

	control = make(chan struct{})
	lock    sync.Mutex

	defaultEntity = &Entity{}
)

func currentPodInfo() (string, string, error) {
	namespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", "", fmt.Errorf("failed get namespace in sidecar mode, err: %v", err)
	}

	name, err := os.ReadFile("/etc/hostname")
	if err != nil {
		return "", "", fmt.Errorf("failed get namespace in sidecar mode, err: %v", err)
	}
	return string(namespace), string(name), nil
}

func initDefaultEntity(sidecarMode bool) error {
	self := os.Getpid()
	hostNetNSId, err := getNsInumByPid(self)
	if err != nil {
		return fmt.Errorf("failed get host nsnum id, err: %w", err)
	}

	ipList, err := hostIPList()
	if err != nil {
		return err
	}

	//add host network
	defaultEntity = &Entity{
		netnsMeta: netnsMeta{
			inum:          hostNetNSId,
			mountPath:     fmt.Sprintf("/proc/%d/ns/net", self),
			isHostNetwork: !sidecarMode,
			ipList:        ipList,
		},
		initPid: self,
	}

	if sidecarMode {
		namespace, name, err := currentPodInfo()
		if err != nil {
			return fmt.Errorf("failed get current pod info: %w", err)
		}

		podIP := ""
		if len(ipList) > 0 {
			podIP = ipList[0]
		}

		defaultEntity.podMeta = podMeta{
			namespace: namespace,
			name:      name,
			ip:        podIP,
		}
	}

	return nil
}

func hostIPList() ([]string, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("failed get host link list: %w", err)
	}

	var ret []string

	for _, link := range links {
		addrs, err := netlink.AddrList(link, unix.AF_INET)
		if err != nil {
			log.Errorf("failed get addr from link %s: %v", link.Attrs().Name, err)
			continue
		}
		for _, addr := range addrs {
			if !addr.IP.IsGlobalUnicast() {
				continue
			}
			ret = append(ret, addr.IP.String())
		}
	}
	return ret, nil
}

type netnsMeta struct {
	inum          int
	mountPath     string
	isHostNetwork bool
	ipList        []string
}

type podMeta struct {
	name      string
	namespace string
	ip        string
}

type Entity struct {
	netnsMeta
	podMeta
	initPid int
	pids    []int
}

func (e *Entity) GetIP() string {
	return e.podMeta.ip
}

func (e *Entity) GetPodName() string {
	return e.podMeta.name
}

func (e *Entity) GetPodNamespace() string {
	return e.podMeta.namespace
}

func (e *Entity) IsHostNetwork() bool {
	return e.netnsMeta.isHostNetwork
}

func (e *Entity) GetNetns() int {
	return e.netnsMeta.inum
}

func (e *Entity) GetNetnsMountPoint() string {
	return e.netnsMeta.mountPath
}

func (e *Entity) GetNsHandle() (netns.NsHandle, error) {
	//TODO check whether we should close the opened file
	return netns.GetFromPath(e.netnsMeta.mountPath)
}

func (e *Entity) GetNetNsFd() (int, error) {
	h, err := e.GetNsHandle()
	if err != nil {
		return InitNetns, err
	}

	return int(h), nil
}

// GetPid return a random initPid of entify, if no process in netns,return 0
func (e *Entity) GetPid() int {
	return e.initPid
}
func (e *Entity) GetPids() []int {
	return e.pids
}

func StartCache(ctx context.Context, sidecarMode bool) error {
	if err := initCriClient(runtimeEndpoints); err != nil {
		return err
	}
	if err := initCriInfo(); err != nil {
		return err
	}

	if err := initDefaultEntity(sidecarMode); err != nil {
		return err
	}
	if sidecarMode {
		return nil
	}

	if err := cachePodsWithTimeout(cacheUpdateInterval); err != nil {
		return fmt.Errorf("failed cache pods, err: %v", err)
	}

	go cacheDaemonLoop(ctx, control)
	return nil
}

func StopCache() {
	control <- struct{}{}
}

func cacheDaemonLoop(_ context.Context, control chan struct{}) {
	log.Infof("nettop cache loop start")

	t := time.NewTicker(cacheUpdateInterval)
	defer t.Stop()

	for {
		select {
		case <-control:
			log.Info("cache daemon loop exit of control signal")
		case <-t.C:
			if err := cachePodsWithTimeout(cacheUpdateInterval); err != nil {
				log.Errorf("failed cache pods: %v", err)
			}
		}
	}

}

func cachePodsWithTimeout(timeout time.Duration) error {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var err error

	cacheDone := make(chan struct{})
	go func(done chan struct{}) {
		err = cacheNetTopology(ctx)
		done <- struct{}{}
	}(cacheDone)

	select {
	case <-ctx.Done():
		log.Infof("cache process time exceeded, latency: %fs", time.Since(start).Seconds())
		return fmt.Errorf("timeout process pods")
	case <-cacheDone:
		log.Infof("cache process finished, latency: %fs", time.Since(start).Seconds())
		return err
	}
}

func addEntityToCache(e *Entity) {
	nsCache.Set(fmt.Sprintf("%d", e.inum), e, 3*cacheUpdateInterval)
	for _, ip := range e.ipList {
		ipCache.Set(ip, e, 3*cacheUpdateInterval)
	}
	for _, pid := range e.pids {
		pidCache.Set(fmt.Sprintf("%d", pid), e, 3*cacheUpdateInterval)
	}
}

func contextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

type CRIInfo struct {
	Version        string
	RuntimeName    string
	RuntimeVersion string
}

func getSandboxInfoSpec(sandboxStatus *v1.PodSandboxStatusResponse) (*sandboxInfoSpec, error) {
	if criInfo.RuntimeName == "docker" {
		return getSandboxInfoSpecForDocker(sandboxStatus.Status.Id)
	}

	infoString := sandboxStatus.Info["info"]
	if infoString == "" {
		return nil, fmt.Errorf("sandbox status does not contains \"info\" field")
	}
	info := &sandboxInfoSpec{}
	if err := json.Unmarshal([]byte(infoString), info); err != nil {
		return nil, fmt.Errorf("failed unmarsh info to struct, err: %v", err)
	}

	return info, nil
}

func cacheNetTopology(ctx context.Context) error {
	lock.Lock()
	defer lock.Unlock()

	addEntityToCache(defaultEntity)

	sandboxList, err := criClient.ListPodSandbox(&v1.PodSandboxFilter{
		State: &v1.PodSandboxStateValue{
			State: v1.PodSandboxState_SANDBOX_READY,
		},
	})

	if err != nil {
		return fmt.Errorf("failed list pod sandboxes: %w", err)
	}

	for _, sandbox := range sandboxList {

		if contextDone(ctx) {
			return fmt.Errorf("timeout")
		}
		if sandbox.Metadata == nil {
			log.Errorf("invalid sandbox who has no metadata, id %s", sandbox.Id)
		}

		namespace := sandbox.Metadata.Namespace
		name := sandbox.Metadata.Name

		sandboxStatus, err := criClient.PodSandboxStatus(sandbox.Id, true)
		if err != nil {
			log.Errorf("sandbox: %s/%s failed get status err: %v", namespace, name, err)
			continue
		}

		if sandboxStatus.Status == nil {
			log.Errorf("sandbox %s/%s: invalid sandbox status", sandbox.Metadata.Namespace, sandbox.Metadata.Name)
			continue
		}

		info, err := getSandboxInfoSpec(sandboxStatus)
		if err != nil {
			log.Errorf("failed get sandbox info: %v", err)
			continue
		}

		netnsNum, err := getNsInumByPid(info.Pid)
		if err != nil {
			log.Errorf("failed get netns for initPid %d, err: %v", info.Pid, err)
			continue
		}

		podCgroupPath := info.Config.Linux.CgroupParent
		var pids []int
		if podCgroupPath != "" {
			pids = tasksInsidePodCgroup(podCgroupPath)
			log.Debugf("found %d pids under cgroup %s", len(pids), podCgroupPath)
		}

		status := sandboxStatus.Status

		if netnsNum == defaultEntity.inum {
			log.Infof("skip host network pod %s/%s", namespace, name)
			continue
		}

		if sandboxStatus.Status.Network == nil || sandboxStatus.Status.Network.Ip == "" {
			log.Errorf("sanbox %s/%s: invalid sandbox status, no ip", sandbox.Metadata.Namespace, sandbox.Metadata.Name)
			continue
		}

		e := &Entity{
			netnsMeta: netnsMeta{
				inum:          netnsNum,
				mountPath:     fmt.Sprintf("/proc/%d/ns/net", info.Pid),
				isHostNetwork: false,
				ipList:        []string{status.Network.Ip},
			},
			podMeta: podMeta{
				name:      name,
				namespace: namespace,
				ip:        sandboxStatus.Status.Network.Ip,
			},
			initPid: info.Pid,
			pids:    pids,
		}

		addEntityToCache(e)
	}

	log.Debug("finished cache process")
	return nil
}
