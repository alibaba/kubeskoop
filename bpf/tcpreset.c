/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore
#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_core_read.h"
#include "bpf_tracing.h"
#include "inspector.h"

#define RESET_NOSOCK 1
#define RESET_ACTIVE 2
#define RESET_PROCESS 4
#define RESET_RECEIVE 8

struct receive_reset_args {
	unsigned long pad;
    const void * skaddr;
    u16 sport;
    u16 dport;
    u8 saddr[4];
    u8 daddr[4];
    u8 saddr_v6[16];
    u8 daddr_v6[16];
    u64 sock_cookie;
}__attribute__((packed));

struct send_reset_args {
	unsigned long pad;
	const void * skbaddr;
    const void * skaddr;
    int state;
    u16 sport;
    u16 dport;
    u8 saddr[4];
    u8 daddr[4];
    u8 saddr_v6[16];
    u8 daddr_v6[16];
};

struct insp_tcpreset_event_t {
	u32 type;
    u8 state;
	struct tuple tuple;
	struct skb_meta skb_meta;
	s64 stack_id;
};

struct insp_tcpreset_event_t *unused_event __attribute__((unused));

struct {
	__uint(type, BPF_MAP_TYPE_STACK_TRACE);
	__uint(max_entries, 1000);
	__uint(key_size, sizeof(u32));
	__uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
} insp_tcpreset_stack SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_tcpreset_events SEC(".maps");

SEC("kprobe/tcp_v4_send_reset")
int trace_sendreset(struct pt_regs * ctx)
{
	struct sock * sk = (struct sock *)PT_REGS_PARM1(ctx);
	struct sk_buff * skb = (struct sk_buff *)(void *)PT_REGS_PARM2(ctx);
	struct insp_tcpreset_event_t event = {0};
	if (sk){
		event.type = RESET_PROCESS;
	}else{
		event.type = RESET_NOSOCK;
	}

	bpf_core_read(&event.state,sizeof(event.state),&sk->__sk_common.skc_state);
	event.stack_id = bpf_get_stackid((struct pt_regs *)ctx, &insp_tcpreset_stack, BPF_F_FAST_STACK_CMP);
	set_tuple(skb, &event.tuple);
	set_meta(skb,&event.skb_meta);

	bpf_perf_event_output((struct pt_regs *)ctx,&insp_tcpreset_events,BPF_F_CURRENT_CPU,&event,sizeof(event));
	return 0;
}


SEC("kprobe/tcp_send_active_reset")
int trace_sendactive(struct pt_regs * ctx)
{
    struct sock * sk = (struct sock *)PT_REGS_PARM1(ctx);
	struct insp_tcpreset_event_t event = {0};
	event.type = RESET_ACTIVE;
	bpf_core_read(&event.state,sizeof(event.state),&sk->__sk_common.skc_state);
	set_tuple_sock(sk,&event.tuple);
	set_meta_sock(sk,&event.skb_meta);
	bpf_perf_event_output((struct pt_regs *)ctx,&insp_tcpreset_events,BPF_F_CURRENT_CPU,&event,sizeof(event));
	return 0;
}

SEC("tracepoint/tcp/tcp_receive_reset")
int insp_rstrx(struct receive_reset_args *ctx)
{
	struct insp_tcpreset_event_t event = {0};
	event.stack_id = bpf_get_stackid((struct pt_regs *)ctx, &insp_tcpreset_stack, BPF_F_FAST_STACK_CMP);
	if ((int)event.stack_id < 0) {
		return 0;
	}
	event.type = RESET_RECEIVE;
	struct sock * sk = (struct sock *)ctx->skaddr;
	bpf_core_read(&event.state,sizeof(event.state),&sk->__sk_common.skc_state);
	set_tuple_sock(sk,&event.tuple);
	set_meta_sock(sk,&event.skb_meta);
	bpf_perf_event_output((struct pt_regs *)ctx,&insp_tcpreset_events,BPF_F_CURRENT_CPU,&event,sizeof(event));
	return 0;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";
