#include <vmlinux.h>
#include <bpf_helpers.h>
#include <bpf_tracing.h>
#include <bpf_core_read.h>
#include <inspector.h>
#include <feature-switch.h>

#define TC_ACT_OK 0

//todo aggregate all flow based metrics in one map to save memory.
struct flow_metrics {
    u64 packets;
    u64 bytes;
    u32 drops;
    u32 retrans;
};

struct {
  __uint(type, BPF_MAP_TYPE_LRU_PERCPU_HASH);
  __type(key, struct flow_tuple_4);
  __type(value, struct flow_metrics);
  __uint(max_entries, 65535);
} insp_flow4_metrics SEC(".maps");

FEATURE_SWITCH(flow)

static inline int __do_flow(struct __sk_buff *skb){
    struct flow_tuple_4 tuple = {0};
    int flow_port_key = 0;
    bool enable_flow_port = is_enable(flow_port_key);

    if(set_flow_tuple4(skb, &tuple, enable_flow_port) < 0){
        goto out;
    }

    struct flow_metrics *metric = bpf_map_lookup_elem(&insp_flow4_metrics, &tuple);
    if(metric){
        __sync_fetch_and_add(&metric->packets, 1);
        __sync_fetch_and_add(&metric->bytes, skb->len);
    }else {
        struct flow_metrics m = {1, skb->len, 0, 0};
        bpf_map_update_elem(&insp_flow4_metrics, &tuple, &m, BPF_ANY);
    }
out:
    return TC_ACT_OK;
}

SEC("tc/ingress")
int tc_ingress(struct __sk_buff *skb){
    return __do_flow(skb);
}

SEC("tc/egress")
int tc_egress(struct __sk_buff *skb){
    return __do_flow(skb);
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";


