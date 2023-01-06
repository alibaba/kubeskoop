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

struct softirq_args {
	unsigned long pad;
    unsigned int vec;
};

struct insp_softirq_event_t {
    u32 pid;
	u32 cpu;
	u32 phase;
    u64 latency;
};

struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, 2);
	__type(key, u32);
	__type(value, u64);
} insp_softirq_entry SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_softirq_events SEC(".maps");

struct insp_softirq_event_t *unused_event __attribute__((unused));

static inline int report_softirq_events(void *ctx, u64 latency, u32 phase)
{
	struct insp_softirq_event_t event = {0};
	event.pid = bpf_get_current_pid_tgid()>> 32;
    event.cpu = bpf_get_smp_processor_id();
	event.latency = latency;
	event.phase = phase;
	bpf_perf_event_output(ctx, &insp_softirq_events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}

SEC("tracepoint/irq/softirq_raise")
int trace_softirq_raise(struct softirq_args *ctx)
{
	u32 vec_nr = ctx->vec;
	if(vec_nr!=3){
        return 0;
    }
	u64 ts = bpf_ktime_get_ns();
	u32 rasiekey = 0;
	bpf_map_update_elem(&insp_softirq_entry, &rasiekey, &ts, 0);
    return 0;
}

SEC("tracepoint/irq/softirq_entry")
int trace_softirq_entry(struct softirq_args *ctx)
{
    u32 vec_nr = ctx->vec;
	if(vec_nr!=3){
        return 0;
    }
	u32 rasiekey = 0;
   	u64 *tsp = bpf_map_lookup_elem(&insp_softirq_entry,&rasiekey);
	if (!tsp) {
        return 0;
    }
	u64 ts = bpf_ktime_get_ns();
    u64 latency;
    latency = ts - *tsp;
	if( latency>SOFTIRQ_THRESH ){
        report_softirq_events(ctx,latency,PHASE_SCHED);
    }
	u32 entrykey = 1;
	bpf_map_update_elem(&insp_softirq_entry, &entrykey, &ts, 0);
	return 0;
}

SEC("tracepoint/irq/softirq_exit")
int trace_softirq_exit(struct softirq_args *ctx)
{
	u32 vec_nr = ctx->vec;
	if(vec_nr!=3){
        return 0;
    }
	u32 entrykey = 1;
	u64 *tsp = bpf_map_lookup_elem(&insp_softirq_entry,&entrykey);
	if (!tsp) {
        return 0;
    }
	u64 ts = bpf_ktime_get_ns();
    u64 latency;
	latency = ts - *tsp;
	if( latency>SOFTIRQ_THRESH ){
        report_softirq_events(ctx,latency,PHASE_EXCUTE);
    }
	return 0;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";
