/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore

#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"

#define RAISE_KEY 1
#define ENTRY_KEY 2

#define PHASE_SCHED 1
#define PHASE_EXCUTE 2
#define SOFTIRQ_THRESH 10000000

volatile const __u32 irq_filter_bits = 0x8; // {"net_rx"}

bool filter_irqs(u32 vec_nr)
{
    return (irq_filter_bits & (1 << vec_nr)) != 0;
}

struct softirq_args {
	unsigned long pad;
    unsigned int vec;
};

struct insp_softirq_entry_key {
    u32 vec_nr;
    u32 phase;
};

struct insp_softirq_event_t {
    u32 pid;
	u32 cpu;
	u32 phase;
	u32 vec_nr;
    u64 latency;
};

struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_HASH);
	__uint(max_entries, 64);
	__type(key, struct insp_softirq_entry_key);
	__type(value, u64);
} insp_softirq_entry SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_softirq_events SEC(".maps");

struct insp_softirq_event_t *unused_event __attribute__((unused));

static inline int report_softirq_events(void *ctx, u64 latency, u32 phase, u32 vec_nr)
{
	struct insp_softirq_event_t event = {0};
	event.pid = bpf_get_current_pid_tgid()>> 32;
	event.latency = latency;
	event.phase = phase;
	event.vec_nr = vec_nr;
	bpf_perf_event_output(ctx, &insp_softirq_events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}

SEC("tracepoint/irq/softirq_raise")
int trace_softirq_raise(struct softirq_args *ctx)
{
	u32 vec_nr = ctx->vec;
	if(filter_irqs(vec_nr) == false){
        return 0;
    }
	u64 ts = bpf_ktime_get_ns();
	struct insp_softirq_entry_key key = {0};
    key.vec_nr = vec_nr;
    key.phase = PHASE_SCHED;
    // only keep the first raise entry
	bpf_map_update_elem(&insp_softirq_entry, &key, &ts, BPF_NOEXIST);
    return 0;
}

SEC("tracepoint/irq/softirq_entry")
int trace_softirq_entry(struct softirq_args *ctx)
{
    u32 vec_nr = ctx->vec;
    if(filter_irqs(vec_nr) == false){
        return 0;
    }
    struct insp_softirq_entry_key key = {0};
    key.vec_nr = vec_nr;
    key.phase = PHASE_SCHED;
    u64 ts = bpf_ktime_get_ns();
   	u64 *tsp = bpf_map_lookup_elem(&insp_softirq_entry, &key);
	if (tsp && *tsp != 0) {
        u64 latency;
        latency = ts - *tsp;
        if( latency>SOFTIRQ_THRESH ){
            report_softirq_events(ctx,latency, PHASE_SCHED, vec_nr);
        }
    }
    bpf_map_delete_elem(&insp_softirq_entry, &key);

    key.vec_nr = vec_nr;
    key.phase = PHASE_EXCUTE;
	bpf_map_update_elem(&insp_softirq_entry, &key, &ts, BPF_ANY);
	return 0;
}

SEC("tracepoint/irq/softirq_exit")
int trace_softirq_exit(struct softirq_args *ctx)
{
	u32 vec_nr = ctx->vec;
	if(filter_irqs(vec_nr) == false){
        return 0;
    }
    struct insp_softirq_entry_key key = {0};
    key.vec_nr = vec_nr;
    key.phase = PHASE_EXCUTE;
	u64 *tsp = bpf_map_lookup_elem(&insp_softirq_entry, &key);
	if (!tsp || *tsp == 0) {
        return 0;
    }
	u64 ts = bpf_ktime_get_ns();
    u64 latency;
	latency = ts - *tsp;
	if( latency>SOFTIRQ_THRESH ){
        report_softirq_events(ctx,latency,PHASE_EXCUTE, vec_nr);
    }
    bpf_map_delete_elem(&insp_softirq_entry, &key);
	return 0;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";
