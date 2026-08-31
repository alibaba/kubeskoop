package sink

import (
    "net/http"
    "sort"
    "strconv"
    "strings"
    "sync"

    "github.com/alibaba/kubeskoop/pkg/exporter/nettop"
    "github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

// FlameAggregator aggregates kernel stacks from events and exposes folded stacks
// suitable for flamegraph generation.
type FlameAggregator struct {
    mu sync.RWMutex
    // data[scopeKey][eventType][folded] = count
    data map[string]map[string]map[string]uint64
    // cached node scope key
    nodeScope string
}

func NewFlameAggregator() *FlameAggregator {
    return &FlameAggregator{
        data:      make(map[string]map[string]map[string]uint64),
        nodeScope: "node:" + nettop.GetNodeName(),
    }
}

func (fa *FlameAggregator) add(scopeKey, eventType, folded string) {
    if folded == "" {
        return
    }
    fa.mu.Lock()
    defer fa.mu.Unlock()
    if _, ok := fa.data[scopeKey]; !ok {
        fa.data[scopeKey] = make(map[string]map[string]uint64)
    }
    if _, ok := fa.data[scopeKey][eventType]; !ok {
        fa.data[scopeKey][eventType] = make(map[string]uint64)
    }
    fa.data[scopeKey][eventType][folded]++
}

func (fa *FlameAggregator) reset(scopeKey, eventType string) {
    fa.mu.Lock()
    defer fa.mu.Unlock()
    if eventType == "" {
        delete(fa.data, scopeKey)
        return
    }
    if _, ok := fa.data[scopeKey]; ok {
        delete(fa.data[scopeKey], eventType)
        if len(fa.data[scopeKey]) == 0 {
            delete(fa.data, scopeKey)
        }
    }
}

// GetCollapsed returns folded stacks lines for given scope and optional event type.
// scopeType: "node" or "pod"
// name: for node scope, empty to use current node; for pod scope, "namespace/pod"
func (fa *FlameAggregator) GetCollapsed(scopeType, name, eventType string) string {
    scopeKey := fa.scopeKey(scopeType, name)
    if scopeKey == "" {
        return ""
    }
    fa.mu.RLock()
    defer fa.mu.RUnlock()
    scopeData, ok := fa.data[scopeKey]
    if !ok {
        return ""
    }
    var lines []string
    if eventType != "" {
        for folded, cnt := range scopeData[eventType] {
            lines = append(lines, folded+" "+strconv.FormatUint(cnt, 10))
        }
    } else {
        for _, m := range scopeData {
            for folded, cnt := range m {
                lines = append(lines, folded+" "+strconv.FormatUint(cnt, 10))
            }
        }
    }
    sort.Strings(lines)
    return strings.Join(lines, "\n")
}

func (fa *FlameAggregator) scopeKey(scopeType, name string) string {
    switch scopeType {
    case "node":
        if name == "" {
            return fa.nodeScope
        }
        return "node:" + name
    case "pod":
        if name == "" {
            return ""
        }
        return "pod:" + name
    default:
        return ""
    }
}

// AddEvent ingests an event and records folded stacks per relevant scopes.
func (fa *FlameAggregator) AddEvent(evt *probe.Event) {
    if evt == nil || evt.Message == "" {
        return
    }
    frames := strings.Split(evt.Message, "\n")
    var stack []string
    for _, f := range frames {
        f = strings.TrimSpace(f)
        if f == "" {
            continue
        }
        // Convert to folded frame token, keep as-is
        // evt.Message already contains "symbol+0xOFFSET"
        stack = append(stack, f)
    }
    if len(stack) == 0 {
        return
    }
    folded := strings.Join(stack, ";")

    // Always add to node scope (current node)
    fa.add(fa.nodeScope, string(evt.Type), folded)

    // Build a quick map of labels
    labels := map[string]string{}
    for _, l := range evt.Labels {
        labels[l.Name] = l.Value
    }
    // For pods on either src/dst side
    if labels["src_type"] == "pod" {
        ns := labels["src_namespace"]
        pod := labels["src_pod"]
        if ns != "" && pod != "" {
            fa.add("pod:"+ns+"/"+pod, string(evt.Type), folded)
        }
    }
    if labels["dst_type"] == "pod" {
        ns := labels["dst_namespace"]
        pod := labels["dst_pod"]
        if ns != "" && pod != "" {
            fa.add("pod:"+ns+"/"+pod, string(evt.Type), folded)
        }
    }
}

// flameSink implements Sink and feeds the default aggregator.
type flameSink struct{}

func (f *flameSink) Write(event *probe.Event) error {
    defaultFlameAgg.AddEvent(event)
    return nil
}

func (f *flameSink) String() string { return "flame" }

func NewFlameSink() *flameSink { return &flameSink{} }

var defaultFlameAgg = NewFlameAggregator()

// HTTP helper used by server to serve collapsed output.
func ServeFlameCollapsed(w http.ResponseWriter, r *http.Request) {
    scope := r.URL.Query().Get("scope") // "node" or "pod"
    eventType := r.URL.Query().Get("event")
    reset := r.URL.Query().Get("reset")

    name := ""
    if scope == "pod" {
        ns := r.URL.Query().Get("namespace")
        pod := r.URL.Query().Get("pod")
        if ns != "" && pod != "" {
            name = ns + "/" + pod
        }
    } else if scope == "node" {
        // optional explicit node name via name= parameter
        name = r.URL.Query().Get("name")
    }

    collapsed := defaultFlameAgg.GetCollapsed(scope, name, eventType)

    // Optionally reset after read
    if reset == "1" || strings.EqualFold(reset, "true") {
        defaultFlameAgg.reset(defaultFlameAgg.scopeKey(scope, name), eventType)
    }

    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    _, _ = w.Write([]byte(collapsed))
}


