package nettop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/fsnotify/fsnotify"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var (
	logger = log.Log.WithName("nettop-k8s")
)

type PodCacheInfo struct {
	UID           string
	IP            string
	Name          string
	Namespace     string
	Labels        map[string]string
	CgroupPath    string
	SandboxPID    int
	NetNSPath     string
	NetNSInode    uint64
	IsHostNetwork bool
}

type cgroupCacheKey struct {
	path  string
	inode uint64
}
type PodCache struct {
	// Pod basic info cache
	podInfoCache     map[string]PodCacheInfo
	podInfoCacheLock sync.RWMutex

	// Cgroup path cache
	cgroupPathCache     map[string]string
	cgroupPathCacheLock sync.RWMutex

	cgroupCache *lru.Cache[cgroupCacheKey, interface{}]
}

func NewPodCache() *PodCache {
	cgroupCache, _ := lru.New[cgroupCacheKey, interface{}](1000)
	return &PodCache{
		podInfoCache:    make(map[string]PodCacheInfo),
		cgroupPathCache: make(map[string]string),
		cgroupCache:     cgroupCache,
	}
}

// isCgroupV2 checks if system is using cgroup v2
func isCgroupV2() bool {
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	return err == nil
}

// getSandboxPID finds sandbox container PID from cgroup tasks
func (pc *PodCache) getSandboxPID(cgroupPath string) (int, error) {
	var earliestTime time.Time
	var sandboxPID int

	err := filepath.Walk(cgroupPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			return nil
		}

		tasksFile := filepath.Join(path, "tasks")
		content, err := os.ReadFile(tasksFile)
		if err != nil {
			return nil
		}

		pids := strings.Split(strings.TrimSpace(string(content)), "\n")
		for _, pidStr := range pids {
			pid, err := strconv.Atoi(pidStr)
			if err != nil {
				continue
			}

			procPath := filepath.Join("/proc", pidStr)
			info, err := os.Stat(procPath)
			if err != nil {
				continue
			}

			cmdlinePath := filepath.Join(procPath, "cmdline")
			cmdline, err := os.ReadFile(cmdlinePath)
			if err != nil {
				continue
			}
			if !strings.Contains(string(cmdline), "pause") {
				continue
			}

			if earliestTime.IsZero() || info.ModTime().Before(earliestTime) {
				earliestTime = info.ModTime()
				sandboxPID = pid
			}
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	if sandboxPID == 0 {
		return 0, fmt.Errorf("sandbox PID not found")
	}

	return sandboxPID, nil
}

// getNetNSInfo gets network namespace path and inode number for a PID
func (pc *PodCache) getNetNSInfo(pid int) (string, uint64, error) {
	nsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
	fi, err := os.Stat(nsPath)
	if err != nil {
		return "", 0, err
	}
	stat := fi.Sys().(*syscall.Stat_t)
	return nsPath, stat.Ino, nil
}

// GetAllLocalPods returns all pods in the cache
func (pc *PodCache) GetAllLocalPods() []PodCacheInfo {
	pc.podInfoCacheLock.RLock()
	defer pc.podInfoCacheLock.RUnlock()

	pods := make([]PodCacheInfo, 0, len(pc.podInfoCache))
	for _, pod := range pc.podInfoCache {
		pods = append(pods, pod)
	}
	return pods
}

// updatePodBasicInfo updates basic pod information in cache
func (pc *PodCache) updatePodBasicInfo(pod *v1.Pod) {
	logger.Info("update pod basic info", "ns", pod.GetNamespace(), "name", pod.Name, "uid", pod.GetUID())
	pc.podInfoCacheLock.Lock()
	defer pc.podInfoCacheLock.Unlock()

	uid := string(pod.UID)

	// Get existing pod info if available
	existingInfo, exists := pc.podInfoCache[uid]

	// Update basic info
	podInfo := PodCacheInfo{
		UID:           uid,
		IP:            pod.Status.PodIP,
		Name:          pod.Name,
		Namespace:     pod.Namespace,
		Labels:        pod.Labels,
		IsHostNetwork: pod.Spec.HostNetwork,
	}

	// Preserve existing cgroup and network namespace info
	if exists {
		podInfo.CgroupPath = existingInfo.CgroupPath
		podInfo.SandboxPID = existingInfo.SandboxPID
		podInfo.NetNSPath = existingInfo.NetNSPath
		podInfo.NetNSInode = existingInfo.NetNSInode
	}

	pc.podInfoCache[uid] = podInfo
}

// updatePodCgroupInfo updates pod's cgroup related information
func (pc *PodCache) updatePodCgroupInfo(uid, cgroupPath string) {
	pathInfo, err := os.Stat(cgroupPath)
	if err != nil {
		logger.Error(err, "error stating cgroup path", "path", cgroupPath)
		return
	}
	pathInode := pathInfo.Sys().(*syscall.Stat_t).Ino
	if pathInode == 0 {
		logger.Error(fmt.Errorf("error stating cgroup path"), "inode is 0")
		return
	}
	_, ok := pc.cgroupCache.Get(cgroupCacheKey{
		path:  cgroupPath,
		inode: pathInode,
	})
	if ok {
		// already updated
		return
	}

	logger.Info("update pod cgroup info", "uid", uid, "cgroupPath", cgroupPath)
	err = wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 1*time.Minute, true, func(_ context.Context) (done bool, err error) {
		task := tasksInsidePodCgroup(cgroupPath, true)
		if len(task) > 0 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		logger.Error(err, "wait pod cgroup info timeout", "cgroupPath", cgroupPath)
	}
	pc.podInfoCacheLock.Lock()
	defer pc.podInfoCacheLock.Unlock()

	// Get existing pod info
	podInfo, exists := pc.podInfoCache[uid]
	if !exists {
		podInfo = PodCacheInfo{
			UID: uid,
		}
	}

	// Update cgroup path
	podInfo.CgroupPath = cgroupPath

	// Try to get sandbox PID and network namespace info
	if sandboxPID, err := pc.getSandboxPID(cgroupPath); err == nil {
		podInfo.SandboxPID = sandboxPID
		if netNSPath, netNSIno, err := pc.getNetNSInfo(sandboxPID); err == nil {
			podInfo.NetNSPath = netNSPath
			podInfo.NetNSInode = netNSIno
			pc.podInfoCache[uid] = podInfo
			pc.cgroupCache.Add(cgroupCacheKey{
				path:  cgroupPath,
				inode: pathInode,
			}, struct{}{})
		} else {
			logger.Error(err, "error getting netns info", "uid", uid, "cgroupPath", cgroupPath)
		}
	}
}

// deletePodFromCache removes a pod from the cache
func (pc *PodCache) deletePodFromCache(uid string) {
	pc.podInfoCacheLock.Lock()
	defer pc.podInfoCacheLock.Unlock()
	logger.Info("delete pod from cache", "uid", uid)
	delete(pc.podInfoCache, uid)
}

// initCgroupWatch initializes cgroup watcher
func (pc *PodCache) initCgroupWatch() {
	var basePath string
	if isCgroupV2() {
		basePath = "/sys/fs/cgroup/kubepods.slice"
	} else {
		basePath = "/sys/fs/cgroup/cpu/kubepods.slice"
	}

	go pc.watchCgroupPath(basePath)
	go pc.watchCgroupPath(filepath.Join(basePath, "kubepods-burstable.slice"))
	go pc.watchCgroupPath(filepath.Join(basePath, "kubepods-besteffort.slice"))
}

// watchCgroupPath watches for changes in cgroup directory
func (pc *PodCache) watchCgroupPath(basePath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	pc.updateCgroupPathCache(basePath)
	err = watcher.Add(basePath)
	if err != nil {
		return
	}

	ticker := time.NewTicker(2 * time.Minute)

	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Create == fsnotify.Create {
				pc.updateCgroupPathCache(basePath)
			}
		case <-ticker.C:
			pc.updateCgroupPathCache(basePath)
		case <-watcher.Errors:
			continue
		}
	}
}

// updateCgroupPathCache updates the mapping of pod UID to cgroup path
func (pc *PodCache) updateCgroupPathCache(basePath string) {
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return
	}

	pc.cgroupPathCacheLock.Lock()
	defer pc.cgroupPathCacheLock.Unlock()

	for _, entry := range entries {
		name := entry.Name()
		var uid string

		switch {
		case strings.HasPrefix(name, "kubepods-burstable-pod"):
			uid = strings.TrimSuffix(strings.TrimPrefix(name, "kubepods-burstable-pod"), ".slice")
		case strings.HasPrefix(name, "kubepods-besteffort-pod"):
			uid = strings.TrimSuffix(strings.TrimPrefix(name, "kubepods-besteffort-pod"), ".slice")
		case strings.HasPrefix(name, "kubepods-pod"):
			uid = strings.TrimSuffix(strings.TrimPrefix(name, "kubepods-pod"), ".slice")
		default:
			continue
		}

		if uid != "" {
			uid = strings.ReplaceAll(uid, "_", "-")
			cgroupPath := filepath.Join(basePath, name)
			pc.cgroupPathCache[uid] = cgroupPath
			// Update pod info with new cgroup path
			go pc.updatePodCgroupInfo(uid, cgroupPath)
		}
	}
}

// StartPodCacheWatch starts watching for pod changes and maintains the cache
func StartPodCacheWatch(ctx context.Context) (*PodCache, error) {
	nodeName := os.Getenv("INSPECTOR_NODENAME")
	if nodeName == "" {
		return nil, fmt.Errorf("INSPECTOR_NODENAME environment variable not set")
	}
	log.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))

	podCache := NewPodCache()

	// Initialize cgroup watcher
	podCache.initCgroupWatch()

	scheme := runtime.NewScheme()
	utilruntime.Must(v1.AddToScheme(scheme))
	// Create manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Cache: cache.Options{
			Scheme: scheme,
			ByObject: map[client.Object]cache.ByObject{
				&v1.Pod{}: {
					Field: client.MatchingFieldsSelector{
						Selector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}),
					},
					Transform: func(i interface{}) (interface{}, error) {
						if pod, ok := i.(*v1.Pod); ok {
							pod.Spec.Volumes = nil
							pod.Spec.EphemeralContainers = nil
							pod.Spec.SecurityContext = nil
							pod.Spec.ImagePullSecrets = nil
							pod.Spec.Tolerations = nil
							pod.Spec.ReadinessGates = nil
							pod.Spec.PreemptionPolicy = nil
							pod.Status.InitContainerStatuses = nil
							pod.Status.ContainerStatuses = nil
							pod.Status.EphemeralContainerStatuses = nil
							return pod, nil
						}
						return nil, fmt.Errorf("unexpected type %T", i)
					},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create manager: %v", err)
	}

	controller := &PodReconciler{PodCache: podCache, Client: mgr.GetClient()}
	// Create pod controller
	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			pod := object.(*v1.Pod)
			return pod.Spec.NodeName == nodeName
		})).
		Complete(controller)
	if err != nil {
		return nil, fmt.Errorf("unable to create controller: %v", err)
	}

	// Start manager
	go func() {
		if err := mgr.Start(ctx); err != nil {
			klog.Errorf("Failed to start manager: %v", err)
		}
	}()

	go controller.GC(ctx)

	return podCache, nil
}

// fixme: add period check

// PodReconciler reconciles Pod objects
type PodReconciler struct {
	client.Client
	PodCache *PodCache
}

func (r *PodReconciler) GC(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			podList := &v1.PodList{}
			err := r.List(ctx, podList)
			if err != nil {
				logger.Error(err, "unable to list pods")
				continue
			}
			podMap := make(map[string]interface{})
			for _, pod := range podList.Items {
				podMap[string(pod.UID)] = struct{}{}
			}
			for _, pod := range r.PodCache.GetAllLocalPods() {
				if _, ok := podMap[pod.UID]; !ok {
					logger.Info("[GC]removing pod from cache", "pod", pod.UID)
					r.PodCache.deletePodFromCache(pod.UID)
				}
			}
		}
	}
}

// Reconcile handles Pod events
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pod := &v1.Pod{}
	err := r.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			r.PodCache.deletePodFromCache(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	r.PodCache.updatePodBasicInfo(pod)
	return ctrl.Result{}, nil
}
