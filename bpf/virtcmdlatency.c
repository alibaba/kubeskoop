/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore
#include <vmlinux.h>
#include <bpf_helpers.h>
#include <bpf_tracing.h>

#define VIRTCMDLAT_THRESH 10000000

struct insp_virtcmdlat_event_t {
    u32 pid;
	u32 cpu;
    u64 latency;
};

struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_virtcmdlat_events SEC(".maps");

struct insp_virtcmdlat_event_t *unused_event __attribute__((unused));

static inline int report_virtcmdlat_events(void *ctx, u64 latency)
{
	struct insp_virtcmdlat_event_t event = {0};
	event.pid = bpf_get_current_pid_tgid() >> 32;
    event.cpu = bpf_get_smp_processor_id();
	event.latency = latency;
	bpf_perf_event_output(ctx, &insp_virtcmdlat_events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}

struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_HASH);
	__uint(max_entries, 1024);
	__type(key, u32);
	__type(value, u64);
} insp_virtcmdlat SEC(".maps");

SEC("kprobe/virtnet_send_command")
int trace_virtcmd()
{
	u32 key = bpf_get_current_pid_tgid();
    u64 ts = bpf_ktime_get_ns();
	bpf_map_update_elem(&insp_virtcmdlat, &key, &ts, BPF_ANY);
	return 0;
}

SEC("kretprobe/virtnet_send_command")
int trace_virtcmdret(struct pt_regs * ctx)
{
	u32 key = bpf_get_current_pid_tgid();
    u64 ts = bpf_ktime_get_ns();
	u64 *tsp;
	tsp = bpf_map_lookup_elem(&insp_virtcmdlat, &key);
	if (tsp) {
		u64 latency = ts - *tsp;
        if (latency > VIRTCMDLAT_THRESH) {
            report_virtcmdlat_events(ctx,latency);
        }
        bpf_map_delete_elem(&insp_virtcmdlat,&key);
	}
	return 0;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";
