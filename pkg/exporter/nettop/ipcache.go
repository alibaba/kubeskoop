package nettop

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
)

type IPType string

const IPTypeNode IPType = "node"
const IPTypePod IPType = "pod"

type IPInfo struct {
	Type         IPType
	IP           string
	NodeName     string
	PodName      string
	PodNamespace string
}

func (i *IPInfo) String() string {
	return fmt.Sprintf("ip=%s,type=%s,node=%s,pod=%s,namespace=%s", i.IP, i.Type, i.NodeName, i.PodName, i.PodNamespace)
}

type IPCache struct {
	period   string
	revision uint64
	entries  map[string]*IPInfo
	lock     sync.RWMutex
}

var cacheHolder = atomic.Pointer[IPCache]{}

func init() {
	cacheHolder.Store(&IPCache{
		period:   "",
		revision: 0,
		entries:  make(map[string]*IPInfo),
	})
}

func GetIPInfo(ip string) *IPInfo {
	cache := cacheHolder.Load()
	cache.lock.RLock()
	defer cache.lock.RUnlock()
	return cache.entries[ip]
}

func IPCacheRevision() (string, uint64) {
	cache := cacheHolder.Load()
	return cache.period, cache.revision
}

func UpdateIPCache(period string, revision uint64, entries []*IPInfo) {
	m := make(map[string]*IPInfo)
	for _, e := range entries {
		m[e.IP] = e
	}

	cache := &IPCache{
		period:   period,
		revision: revision,
		entries:  m,
	}

	cacheHolder.Store(cache)
}

func ApplyIPCacheChange(revision uint64, op rpc.OpCode, info *IPInfo) {
	cache := cacheHolder.Load()
	cache.lock.Lock()
	defer cache.lock.Unlock()
	cache.revision = revision
	switch op {
	case rpc.OpCode_Set:
		cache.entries[info.IP] = info
	case rpc.OpCode_Del:
		delete(cache.entries, info.IP)
	}
}
