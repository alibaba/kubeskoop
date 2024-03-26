/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore

#include <vmlinux.h>
#include <bpf_helpers.h>
#include <bpf_tracing.h>
#include <bpf_core_read.h>
#include <inspector.h>

struct tp_tcp_retransmit_skb_args {
    unsigned long long unused;

    void *skbaddr;
    void *skaddr;
    int state;
    u16 sport;
    u16 dport;
    u8 saddr[4];
    u8 daddr[4];
    u8 saddr_v6[16];
    u8 daddr_v6[16];
};

struct insp_tcpretrans_event_t {
  struct tuple tuple;
  s64 stack_id;
};

const struct insp_tcpretrans_event_t *unused_insp_tcpretrans_event_t __attribute__((unused));

struct {
	__uint(type, BPF_MAP_TYPE_STACK_TRACE);
	__uint(key_size, sizeof(u32));
	__uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
	__uint(max_entries, 1000);
} insp_tcp_retrans_stack SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_tcp_retrans_event SEC(".maps");

SEC("tracepoint/tcp/tcp_retransmit_skb")
int tcpretrans(struct tp_tcp_retransmit_skb_args *args) {
  struct insp_tcpretrans_event_t event = {0};

  struct tuple *tuple = &event.tuple;
  bpf_probe_read_kernel(&tuple->sport, sizeof(tuple->sport), &args->sport);
  bpf_probe_read_kernel(&tuple->dport, sizeof(tuple->dport), &args->dport);
  event.tuple.l4_proto = IPPROTO_TCP;

  bpf_probe_read_kernel(&tuple->saddr.v6addr, sizeof(tuple->saddr.v6addr), &args->saddr_v6);
  bpf_probe_read_kernel(&tuple->daddr.v6addr, sizeof(tuple->daddr.v6addr), &args->daddr_v6);

  event.stack_id = bpf_get_stackid(args, &insp_tcp_retrans_stack,
                                KERN_STACKID_FLAGS);
  bpf_perf_event_output(args, &insp_tcp_retrans_event,
                        BPF_F_CURRENT_CPU, &event, sizeof(event));

out:
  return 0;
}

char _license[] SEC("license") = "GPL";
