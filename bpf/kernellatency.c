/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore
#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_core_read.h"
#include "bpf_tracing.h"
#include "inspector.h"

#define RX_KLATENCY 1
#define TX_KLATENCY 2
#define THRESH 10000000


struct txlatency_t {
    u64 queuexmit;
	u64 local;
	u64 output;
	u64 finish;
};

struct rxlatency_t {
	u64 rcv;
	u64 rcvfinish;
	u64 local;
	u64 localfinish;
};

struct insp_kl_event_t {
    char target[TASK_COMM_LEN];
	struct tuple tuple;
	struct skb_meta skb_meta;
	u32 pid;
	u32 cpu;
	u32 direction;
	u64 latency;
	u64 point1;
	u64 point2;
	u64 point3;
	u64 point4;
};

// struct insp_kernelrx_event_t {
//     char target[TASK_COMM_LEN];
// 	struct tuple tuple;
// 	struct skb_meta skb_meta;
// 	u32 pid;
// 	u32 cpu;
// 	struct rxlatency_t latency;
// 	s64 stack_id;
// };

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, struct sk_buff *);
	__type(value, struct rxlatency_t);
	__uint(max_entries, 10000);
} insp_kernelrx_entry SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, struct sk_buff *);
	__type(value, struct txlatency_t);
	__uint(max_entries, 10000);
} insp_kerneltx_entry SEC(".maps");


struct {
	__uint(type, BPF_MAP_TYPE_STACK_TRACE);
	__uint(max_entries, 1000);
	__uint(key_size, sizeof(u32));
	__uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
} insp_klatency_stack SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_klatency_event SEC(".maps");


struct insp_kl_event_t *unused_event __attribute__((unused));

static inline int update_rxlat(void *ctx, struct sk_buff * skb, struct rxlatency_t * lat )
{
	if (lat->localfinish > lat->rcv) {
		u64 latency;
		latency = lat->localfinish - lat->rcv;
		if (latency > THRESH) {
			struct insp_kl_event_t event = {0};
			bpf_get_current_comm(&event.target, sizeof(event.target));
			set_tuple(skb, &event.tuple);
			set_meta(skb,&event.skb_meta);
			event.pid = bpf_get_current_pid_tgid() >> 32;
            event.cpu = bpf_get_smp_processor_id();
			event.direction = RX_KLATENCY;
			event.latency = latency;
			bpf_probe_read(&event.point1,sizeof(event.point1),&lat->rcv);
			bpf_probe_read(&event.point2,sizeof(event.point1),&lat->rcvfinish);
			bpf_probe_read(&event.point3,sizeof(event.point1),&lat->local);
			bpf_probe_read(&event.point4,sizeof(event.point1),&lat->localfinish);
			bpf_perf_event_output(ctx, &insp_klatency_event, BPF_F_CURRENT_CPU, &event, sizeof(event));
		}
	}
    return 0;
}

static inline int update_txlat(void *ctx, struct sk_buff * skb, struct txlatency_t * lat )
{
	if (lat->finish > lat->queuexmit) {
		u64 latency;
		latency = lat->finish - lat->queuexmit;
		if (latency > THRESH) {
			struct insp_kl_event_t event = {0};
			bpf_get_current_comm(&event.target, sizeof(event.target));
			set_tuple(skb, &event.tuple);
			set_meta(skb,&event.skb_meta);
			event.pid = bpf_get_current_pid_tgid()>> 32;
            event.cpu = bpf_get_smp_processor_id();
			event.direction = TX_KLATENCY;
			event.latency = latency;
			bpf_probe_read(&event.point1,sizeof(event.point1),&lat->queuexmit);
			bpf_probe_read(&event.point2,sizeof(event.point1),&lat->local);
			bpf_probe_read(&event.point3,sizeof(event.point1),&lat->output);
			bpf_probe_read(&event.point4,sizeof(event.point1),&lat->finish);
			// bpf_core_read(&event.latency,sizeof(event.latency),&lat);
			bpf_perf_event_output(ctx, &insp_klatency_event, BPF_F_CURRENT_CPU, &event, sizeof(event));
		}
	}
    return 0;
}

SEC("kprobe/ip_rcv")
int klatency_ip_rcv(struct pt_regs * ctx){
	struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM1(ctx);
	struct rxlatency_t  lat = {0};
	lat.rcv = bpf_ktime_get_ns();
    bpf_map_update_elem(&insp_kernelrx_entry, &skb, &lat, BPF_ANY);
    return 0;
}

SEC("kprobe/ip_rcv_finish")
int klatency_ip_rcv_finish(struct pt_regs * ctx){
    struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM3(ctx);
	struct rxlatency_t * lat = bpf_map_lookup_elem(&insp_kernelrx_entry,&skb);
	if (lat) {
		lat->rcvfinish = bpf_ktime_get_ns();
		// bpf_printk("iprcv %llu iprcvfinish %llu\n",lat->rcv,lat->rcvfinish);
	}
    return 0;
}

SEC("kprobe/ip_local_deliver")
int klatency_ip_local_deliver(struct pt_regs * ctx){
    struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM1(ctx);
	struct rxlatency_t * lat = bpf_map_lookup_elem(&insp_kernelrx_entry,&skb);
	if (lat) {
		lat->local = bpf_ktime_get_ns();
	}

    return 0;
}

SEC("kprobe/ip_local_deliver_finish")
int klatency_ip_local_deliver_finish(struct pt_regs * ctx){
    struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM3(ctx);
	struct rxlatency_t * lat = bpf_map_lookup_elem(&insp_kernelrx_entry,&skb);
	if (lat) {
		lat->localfinish = bpf_ktime_get_ns();
		update_rxlat(ctx,skb,lat);
		bpf_map_delete_elem(&insp_kernelrx_entry, &skb);
	}
    return 0;
}

SEC("kprobe/__ip_queue_xmit")
int klatency_ip_queue_xmit(struct pt_regs * ctx){
	struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM2(ctx);
	struct txlatency_t  lat = {0};
	lat.queuexmit = bpf_ktime_get_ns();
	bpf_map_update_elem(&insp_kerneltx_entry, &skb, &lat, BPF_ANY);
    return 0;
}

SEC("kprobe/ip_local_out")
int klatency_ip_local(struct pt_regs * ctx){
	struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM3(ctx);
	struct txlatency_t * lat = bpf_map_lookup_elem(&insp_kerneltx_entry,&skb);
	if (lat) {
		lat->local = bpf_ktime_get_ns();
	}
    return 0;
}

SEC("kprobe/ip_output")
int klatency_ip_output(struct pt_regs * ctx){
	struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM3(ctx);
	struct txlatency_t * lat = bpf_map_lookup_elem(&insp_kerneltx_entry,&skb);
	if (lat) {
		lat->output = bpf_ktime_get_ns();
	}
    return 0;
}

SEC("kprobe/ip_finish_output2")
int klatency_ip_finish_output2(struct pt_regs * ctx){
    struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM3(ctx);
	struct txlatency_t * lat = bpf_map_lookup_elem(&insp_kerneltx_entry,&skb);
	if (lat) {
		lat->finish = bpf_ktime_get_ns();
		update_txlat(ctx,skb,lat);
		bpf_map_delete_elem(&insp_kerneltx_entry, &skb);
	}
    return 0;
}

SEC("kprobe/kfree_skb")
int report_kfree(struct pt_regs *ctx){
	struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM1(ctx);
	struct rxlatency_t * latrx = bpf_map_lookup_elem(&insp_kernelrx_entry,&skb);
	if (latrx) {
		bpf_map_delete_elem(&insp_kernelrx_entry, &skb);
	}
	struct txlatency_t * lattx = bpf_map_lookup_elem(&insp_kerneltx_entry,&skb);
	if (lattx) {
		bpf_map_delete_elem(&insp_kerneltx_entry, &skb);
	}
	return 0;
}

SEC("kprobe/consume_skb")
int report_consume(struct pt_regs *ctx){
	struct sk_buff * skb = (struct sk_buff *)PT_REGS_PARM1(ctx);
	struct rxlatency_t * latrx = bpf_map_lookup_elem(&insp_kernelrx_entry,&skb);
	if (latrx) {
		bpf_map_delete_elem(&insp_kernelrx_entry, &skb);
	}
	struct txlatency_t * lattx = bpf_map_lookup_elem(&insp_kerneltx_entry,&skb);
	if (lattx) {
		bpf_map_delete_elem(&insp_kerneltx_entry, &skb);
	}
	return 0;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";
