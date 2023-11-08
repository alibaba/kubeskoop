/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore

#include <vmlinux.h>
#include <bpf_helpers.h>
#include <bpf_tracing.h>
#include <bpf_core_read.h>
#include <inspector.h>

#define THRESH (100*1000*1000)

#define ACTION_QDISC	    1
#define ACTION_XMIT	        2

struct net_dev_queue_args {
	unsigned long long unused;
	void * skbaddr;
    unsigned int len;
    int name;
};

struct net_dev_start_xmit_args {
	unsigned long long unused;
	int name;
    u16 queue_mapping;
    const void * skbaddr;
    bool vlan_tagged;
    u16 vlan_proto;
    u16 vlan_tci;
    u16 protocol;
    u8 ip_summed;
    unsigned int len;
    unsigned int data_len;
    int network_offset;
    bool transport_offset_valid;
    int transport_offset;
    u8 tx_flags;
    u16 gso_size;
    u16 gso_segs;
    u16 gso_type;
};

struct insp_nftxlat_event_t {
    char target[TASK_COMM_LEN];
	u32 type;
	struct tuple tuple;
	struct skb_meta skb_meta;
	u32 pid;
	u32 cpu;
	u64 latency;
	s64 stack_id;
};

struct insp_nftxlat_metric_t {
  u32 netns;
  u32 bucket;
  u32 action;
  u32 cpu;
};

struct net_dev_xmit_args {
	unsigned long long unused;
	void * skbaddr;
    unsigned int len;
    int rc;
    int name;
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 4096);
	__type(key, struct insp_nftxlat_metric_t);
	__type(value, u64);
} insp_sklat_metric SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, struct sk_buff *);
	__type(value, u64);
	__uint(max_entries, 10000);
} insp_txq SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, struct sk_buff *);
	__type(value, u64);
	__uint(max_entries, 10000);
} insp_txs SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_sklat_event SEC(".maps");

struct insp_nftxlat_event_t *unused_event __attribute__((unused));

static inline int update_txlat_counts(void *ctx)
{
    return 0;
}

static inline int report_txlat_events(void *ctx, struct sk_buff * skb, u64 latency, u32 action)
{
    struct insp_nftxlat_event_t event = {};
    event.type = action;
    bpf_get_current_comm(&event.target, sizeof(event.target));
    event.pid = bpf_get_current_pid_tgid()>> 32;
    event.cpu = bpf_get_smp_processor_id();
    set_tuple(skb, &event.tuple);
	set_meta(skb,&event.skb_meta);
	event.latency = latency;
	bpf_perf_event_output(ctx, &insp_sklat_event, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}

SEC("tracepoint/net/net_dev_queue")
int net_dev_queue(struct net_dev_queue_args *ctx)
{
	u64 ts = bpf_ktime_get_ns();
	struct sk_buff * skb = (struct sk_buff *)ctx->skbaddr;
	//bpf_printk("tp_btf/net_dev_queue skb = %x\n", skb);
	bpf_map_update_elem(&insp_txq, &skb, &ts, 0);
    return 0;
}

SEC("tracepoint/net/net_dev_start_xmit")
int net_dev_start_xmit(struct net_dev_start_xmit_args *ctx)
{
    struct sk_buff * skb = (struct sk_buff *)ctx->skbaddr;

    u64 *tsp = bpf_map_lookup_elem(&insp_txq,&skb);
    if (!tsp) {
        return 0;
    }
    u64 ts = bpf_ktime_get_ns();
    u64 latency;
    latency = ts - *tsp;
    if( latency>THRESH ){
        report_txlat_events(ctx,skb,latency,ACTION_QDISC);
    }
    bpf_map_update_elem(&insp_txs, &skb, &ts, 0);
    return 0;
}

SEC("tracepoint/net/net_dev_xmit")
int net_dev_xmit(struct net_dev_xmit_args *ctx)
{
    struct sk_buff * skb = (struct sk_buff *)ctx->skbaddr;

    u64 *tsp = bpf_map_lookup_elem(&insp_txs,&skb);
    if (!tsp) {
        return 0;
    }
    u64 ts = bpf_ktime_get_ns();
    u64 latency;
    latency = ts - *tsp;
    if( latency>THRESH ){
        report_txlat_events(ctx,skb,latency,ACTION_XMIT);
    }
    bpf_map_delete_elem(&insp_txs, &skb);
    return 0;
}

char __license[] SEC("license") = "Dual MIT/GPL";
