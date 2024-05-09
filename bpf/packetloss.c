/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore

#include <vmlinux.h>
#include <bpf_helpers.h>
#include <bpf_tracing.h>
#include <bpf_core_read.h>
#include <inspector.h>
#include <feature-switch.h>

struct kfree_skb_args {
  /* The first 8 bytes is not allowed to read */
  unsigned long pad;

  void *skb;
  void *location;
  unsigned short protocol;
};

struct insp_pl_event_t {
  struct tuple tuple;
  u64 location;
  s64 stack_id;
};

const struct insp_pl_event_t *unused_insp_pl_event_t __attribute__((unused));

struct {
	__uint(type, BPF_MAP_TYPE_STACK_TRACE);
	__uint(key_size, sizeof(u32));
	__uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
	__uint(max_entries, 1000);
} insp_pl_stack SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_pl_event SEC(".maps");

FEATURE_SWITCH(packetloss)

SEC("tracepoint/skb/kfree_skb")
int kfree_skb(struct kfree_skb_args *args) {
  struct sk_buff *skb = (struct sk_buff *)args->skb;
  struct insp_pl_event_t event = {0};

  if (set_tuple(skb, &event.tuple) < 0){
    //invalid packet, skip
    goto out;
  }
  event.location = (u64)args->location;

  int packetloss_stack_key = 0;
  bool enable_packetloss_stack = is_enable(packetloss_stack_key);

  if (enable_packetloss_stack){
      event.stack_id = bpf_get_stackid(args, &insp_pl_stack,
                                    KERN_STACKID_FLAGS);
  }

  bpf_perf_event_output(args, &insp_pl_event,
                        BPF_F_CURRENT_CPU, &event, sizeof(event));

out:
  return 0;
}

char _license[] SEC("license") = "GPL";
