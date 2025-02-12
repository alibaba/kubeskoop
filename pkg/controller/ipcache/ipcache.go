package ipcache

import (
	"container/list"
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	MaxSyncBatchSize = 20
	CompactThreshold = 10240
)

type changeLog struct {
	revision uint64
	opcode   rpc.OpCode
	entry    *rpc.CacheEntry
}

type snapshot struct {
	lock     sync.RWMutex
	revision uint64
	entries  map[string]*rpc.CacheEntry
}

type storage struct {
	revision atomic.Uint64
	snapshot snapshot
	//full     map[string]*rpc.CacheEntry
	log []*changeLog
}

type clientstate int

const (
	stale    clientstate = 0
	uptodate clientstate = 1
	blocking clientstate = 2
)

type client struct {
	revision uint64
	closed   bool
	state    clientstate
	ch       chan *changeLog
}

type Service struct {
	rpc.UnimplementedIPCacheServiceServer
	storage     storage
	clients     *list.List
	clientsLock sync.RWMutex
	period      string
	logChan     chan *changeLog
	logLock     sync.Mutex
}

func (s *Service) ListCache(_ context.Context, _ *rpc.ListCacheRequest) (*rpc.ListCacheResponse, error) {
	s.storage.snapshot.lock.RLock()
	defer s.storage.snapshot.lock.RUnlock()

	var entries []*rpc.CacheEntry
	for _, v := range s.storage.snapshot.entries {
		entries = append(entries, v)
	}

	return &rpc.ListCacheResponse{
		Period:   s.period,
		Revision: s.storage.snapshot.revision,
		Entries:  entries,
	}, nil
}

func (s *Service) WatchCache(request *rpc.WatchCacheRequest, server rpc.IPCacheService_WatchCacheServer) error {
	if request.Period != s.period {
		return status.Error(codes.InvalidArgument, "invalid period")
	}

	s.clientsLock.Lock()
	s.storage.snapshot.lock.RLock()

	if request.Revision < s.storage.snapshot.revision {
		s.storage.snapshot.lock.RUnlock()
		s.clientsLock.Unlock()
		return status.Error(codes.DataLoss, "data loss")
	}

	c := &client{
		revision: request.Revision,
		closed:   false,
		state:    stale,
		ch:       make(chan *changeLog, 4*1024),
	}

	s.clients.PushBack(c)

	s.storage.snapshot.lock.RUnlock()
	s.clientsLock.Unlock()

loop:
	for {
		select {
		case <-server.Context().Done():
			c.closed = true
			break loop
		case cl := <-c.ch:
			err := server.Send(&rpc.WatchCacheResponse{
				Revision: cl.revision,
				Opcode:   cl.opcode,
				Entry:    cl.entry,
			})
			if err != nil {
				log.Warningf("failed send watch response to client: %v", err)
				c.closed = true
				break loop
			}
		}
	}

	return nil
}

func findNext(slice []*changeLog, target uint64) int {
	start := 0
	end := len(slice) - 1
	ret := -1
	for start <= end {
		mid := (start + end) / 2
		if slice[mid].revision <= target {
			start = mid + 1
		} else {
			ret = mid
			end = mid - 1
		}
	}
	return ret
}

func NewService(podInformer coreinformers.PodInformer, nodeInformer coreinformers.NodeInformer) *Service {
	s := &Service{
		storage: storage{
			snapshot: snapshot{
				entries: make(map[string]*rpc.CacheEntry),
			},
		},
		logChan: make(chan *changeLog, 10*1024),
		clients: list.New(),
		period:  uuid.NewString(),
	}
	_, err := podInformer.Informer().AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc:    s.onAddPod,
		DeleteFunc: s.onDeletePod,
		UpdateFunc: s.onUpdatePod,
	})
	if err != nil {
		log.Fatalf("failed to add pod resource handler: %v", err)
	}

	_, err = nodeInformer.Informer().AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc:    s.onAddNode,
		DeleteFunc: s.onDeleteNode,
		UpdateFunc: s.onUpdateNode,
	})
	if err != nil {
		log.Fatalf("failed to add node resource handler: %v", err)
	}

	go s.syncControl()

	return s
}

func allPodIPs(pod *v1.Pod) []string {
	var ipList []string
	if pod.Status.PodIPs != nil {
		for _, ip := range pod.Status.PodIPs {
			ipList = append(ipList, ip.IP)
		}
	} else if pod.Status.PodIP != "" {
		ipList = append(ipList, pod.Status.PodIP)
	} else {
		log.Infof("pod %s/%s has no ip, skip", pod.Namespace, pod.Name)
		return nil
	}
	return ipList
}

func createCacheEntries4Pod(pod *v1.Pod) []*rpc.CacheEntry {
	if pod.Spec.HostNetwork {
		return nil
	}

	var entries []*rpc.CacheEntry

	for _, ip := range allPodIPs(pod) {
		entries = append(entries, &rpc.CacheEntry{
			IP: ip,
			Meta: &rpc.CacheEntry_Pod{
				Pod: &rpc.PodMeta{
					Namespace: pod.Namespace,
					Name:      pod.Name,
				},
			},
		})
	}
	return entries
}

func createCacheEntries4Node(node *v1.Node) []*rpc.CacheEntry {
	var entries []*rpc.CacheEntry
	for _, ip := range allNodeIPs(node) {
		entries = append(entries, &rpc.CacheEntry{
			IP: ip,
			Meta: &rpc.CacheEntry_Node{
				Node: &rpc.NodeMeta{
					Name: node.Name,
				},
			},
		})
	}
	return entries
}

func allNodeIPs(node *v1.Node) []string {
	var ret []string
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP || addr.Type == v1.NodeExternalIP {
			ret = append(ret, addr.Address)
		}
	}
	return ret
}

func (s *Service) onAddPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	entries := createCacheEntries4Pod(pod)
	if len(entries) > 0 {
		s.logChange(rpc.OpCode_Set, entries)
	}
}

func (s *Service) onDeletePod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		pod, ok = tombstone.Obj.(*v1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a pod %#v", obj))
			return
		}
	}

	entries := createCacheEntries4Pod(pod)
	if len(entries) == 0 {
		return
	}

	s.logChange(rpc.OpCode_Del, entries)
}

func (s *Service) onUpdatePod(old interface{}, cur interface{}) {
	newPod := cur.(*v1.Pod)
	oldPod := old.(*v1.Pod)
	if newPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}

	oldIPs := allPodIPs(oldPod)
	newIPs := allPodIPs(newPod)

	if reflect.DeepEqual(oldIPs, newIPs) {
		return
	}

	toRemove := createCacheEntries4Pod(oldPod)
	if len(toRemove) > 0 {
		s.logChange(rpc.OpCode_Del, toRemove)
	}
	toAdd := createCacheEntries4Pod(newPod)
	if len(toAdd) > 0 {
		s.logChange(rpc.OpCode_Set, toAdd)
	}
}

func (s *Service) onAddNode(obj interface{}) {
	node := obj.(*v1.Node)
	entries := createCacheEntries4Node(node)
	if len(entries) > 0 {
		s.logChange(rpc.OpCode_Set, entries)
	}
}

func (s *Service) onDeleteNode(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		node, ok = tombstone.Obj.(*v1.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a pod %#v", obj))
			return
		}
	}

	entries := createCacheEntries4Node(node)
	if len(entries) > 0 {
		s.logChange(rpc.OpCode_Del, entries)
	}
}

func (s *Service) onUpdateNode(old interface{}, cur interface{}) {
	newNode := cur.(*v1.Node)
	oldNode := old.(*v1.Node)
	if newNode.ResourceVersion == oldNode.ResourceVersion {
		return
	}

	oldIPs := allNodeIPs(oldNode)
	newIPs := allNodeIPs(newNode)

	if reflect.DeepEqual(oldIPs, newIPs) {
		return
	}

	toRemove := createCacheEntries4Node(oldNode)
	if len(toRemove) > 0 {
		s.logChange(rpc.OpCode_Del, toRemove)
	}
	toAdd := createCacheEntries4Node(newNode)
	if len(toAdd) > 0 {
		s.logChange(rpc.OpCode_Set, toAdd)
	}
}

func (s *Service) logChange(op rpc.OpCode, entries []*rpc.CacheEntry) {
	if len(entries) == 0 {
		return
	}

	s.logLock.Lock()
	defer s.logLock.Unlock()

	for _, e := range entries {
		revision := s.storage.revision.Add(1)
		s.logChan <- &changeLog{
			revision: revision,
			opcode:   op,
			entry:    e,
		}
	}
}

func apply(m map[string]*rpc.CacheEntry, cl *changeLog) {
	entry := cl.entry
	switch cl.opcode {
	case rpc.OpCode_Set:
		m[entry.IP] = entry
	case rpc.OpCode_Del:
		delete(m, entry.IP)
	default:
		log.Errorf("invalid opcode %d", cl.opcode)
	}
}

func (s *Service) compactLog() {
	s.clientsLock.RLock()
	defer s.clientsLock.RUnlock()

	s.storage.snapshot.lock.Lock()
	defer s.storage.snapshot.lock.Unlock()

	var minRevision uint64

	for e := s.clients.Front(); e != nil; e = e.Next() {
		c := e.Value.(*client)
		if c.closed {
			continue
		}
		if c.revision < minRevision {
			minRevision = c.revision
		}
	}

	if minRevision == 0 {
		return
	}

	var first, last uint64

	for i := 0; i < len(s.storage.log); {
		cl := s.storage.log[i]
		if cl.revision > minRevision {
			break
		}
		if first == 0 {
			first = cl.revision
		}
		last = cl.revision
		apply(s.storage.snapshot.entries, cl)
		s.storage.snapshot.revision = cl.revision
	}

	if last > 0 {
		log.Infof("compact log from %d(include) to %d(include)", first, last)
	}
}

func (s *Service) copyClients(filter ...clientstate) []*client {
	s.clientsLock.RLock()
	defer s.clientsLock.RUnlock()

	var ret []*client
	for e := s.clients.Front(); e != nil; e = e.Next() {
		c := e.Value.(*client)
		if c.closed {
			close(c.ch)
			s.clients.Remove(e)
			continue
		}
		if len(filter) == 0 || slices.Contains(filter, c.state) {
			ret = append(ret, c)
		}
	}
	return ret
}

func (s *Service) syncControl() {
	ticker := time.NewTicker(500 * time.Millisecond)

	var pendingLogs []*changeLog

	var maxSyncedRevision uint64

	for {
		select {
		case cl := <-s.logChan:
			//TODO check change log effectiveness, if change log has no effect on current data, ignore it to reduce sync

			s.storage.log = append(s.storage.log, cl)
			if len(s.storage.log) > CompactThreshold {
				s.compactLog()
			}

			pendingLogs = append(pendingLogs, cl)
			if len(pendingLogs) > MaxSyncBatchSize {
				s.syncUpToDateClients(pendingLogs, maxSyncedRevision)
				maxSyncedRevision = pendingLogs[len(pendingLogs)-1].revision
				pendingLogs = nil
			}
		case <-ticker.C:
			if len(pendingLogs) > 0 {
				s.syncUpToDateClients(pendingLogs, maxSyncedRevision)
				maxSyncedRevision = pendingLogs[len(pendingLogs)-1].revision
				pendingLogs = nil
			}

			s.syncClients(maxSyncedRevision)
		}
	}
}

func (s *Service) syncUpToDateClients(cls []*changeLog, upToDateRevision uint64) {
	clients := s.copyClients(uptodate)
	for _, c := range clients {
		if c.revision != upToDateRevision {
			panic(fmt.Sprintf("expect revision %d for up to date client, but got %d", upToDateRevision, c.revision))
		}
		s.syncOneUpdateToDateClient(c, cls)
	}
}

func (s *Service) syncOneUpdateToDateClient(c *client, cls []*changeLog) {
	for _, cl := range cls {
		select {
		case c.ch <- cl:
			c.revision = cl.revision
		default:
			c.state = blocking
			return
		}
	}

	c.state = uptodate
}

func (s *Service) syncClients(upToDateRevision uint64) {
	clients := s.copyClients(stale, blocking)
	for _, c := range clients {
		s.syncOne(c, upToDateRevision)
	}
}

func (s *Service) syncOne(c *client, upToDateRevision uint64) {
	index := findNext(s.storage.log, c.revision)

	if index == -1 {
		return
	}

	for index < len(s.storage.log) {
		cl := s.storage.log[index]
		select {
		case c.ch <- cl:
			index++
			c.revision = cl.revision
			if cl.revision == upToDateRevision {
				c.state = uptodate
				return
			}
			if cl.revision > upToDateRevision {
				//impossible
				c.state = stale
				return
			}
		default:
			c.state = blocking
			return
		}
	}
}

var _ rpc.IPCacheServiceServer = &Service{}
