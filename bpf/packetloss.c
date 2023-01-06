/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore

#include <vmlinux.h>
#include <bpf_helpers.h>
#include <bpf_tracing.h>
#include <bpf_core_read.h>
#include <inspector.h>

struct kfree_skb_args {
  /* The first 8 bytes is not allowed to read */
  unsigned long pad;

  void *skb;
  void *location;
  unsigned short protocol;
};

struct insp_pl_event_t {
  char target[TASK_COMM_LEN];
  struct tuple tuple;
  struct skb_meta skb_meta;
  u32 pid;
  u32 cpu;
  u64 location;
  s64 stack_id;
};

struct insp_pl_metric_t {
  u64 location;
  u32 netns;
  u8 protocol;
};

struct insp_pl_event_t *unused_event __attribute__((unused));
struct insp_pl_metric_t *unused_event2 __attribute__((unused));

struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __type(key, struct insp_pl_metric_t);
  __type(value, u64);
  __uint(max_entries, 4096);
} insp_pl_metric SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_STACK_TRACE);
	__uint(key_size, sizeof(u32));
	__uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
	__uint(max_entries, 1000);
} insp_pl_stack SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_pl_event SEC(".maps");

SEC("tracepoint/skb/kfree_skb")
int kfree_skb(struct kfree_skb_args *args) {
  struct sk_buff *skb = (struct sk_buff *)args->skb;
  struct insp_pl_metric_t mkey = {0};
  struct insp_pl_event_t event = {0};

  set_tuple(skb, &event.tuple);
  set_meta(skb, &event.skb_meta);

  mkey.netns = get_netns(skb);
  mkey.location = (u64)args->location;
  mkey.protocol = (u8)args->protocol;
  u64 *valp = bpf_map_lookup_elem(&insp_pl_metric, &mkey);
  if (!valp) {
    u64 initval = 1;
    bpf_map_update_elem(&insp_pl_metric, &mkey, &initval, 0);
  } else {
    __sync_fetch_and_add(valp, 1);
  }

  bpf_get_current_comm(&event.target, sizeof(event.target));
  event.pid = bpf_get_current_pid_tgid()>> 32;
  event.cpu = bpf_get_smp_processor_id();
  event.location = (u64)args->location;
  event.stack_id = bpf_get_stackid((struct pt_regs *)args, &insp_pl_stack,
                                KERN_STACKID_FLAGS);
  bpf_perf_event_output((struct pt_regs *)args, &insp_pl_event,
                        BPF_F_CURRENT_CPU, &event, sizeof(event));

out:
  return 0;
}

char _license[] SEC("license") = "GPL";
