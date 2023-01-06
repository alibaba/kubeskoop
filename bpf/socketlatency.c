/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore

#include <vmlinux.h>
#include <bpf_helpers.h>
#include <bpf_tracing.h>
#include <bpf_core_read.h>
#include <inspector.h>

char _license[] SEC("license") = "GPL";

#define LAT_THRESH_NS	(1000*1000)
#define LAT_THRESH_NS_10MS	(10*1000*1000)
#define LAT_THRESH_NS_100MS	(100*1000*1000)
#define ACTION_READ	    1
#define ACTION_WRITE	2
#define ACTION_HANDLE	4

#define BUCKET300MS 8
#define BUCKET100MS 1
#define BUCKET10MS  2
#define BUCKET1MS   4

struct sklat_key_t {
	u64 createat;
	u64 lastreceive;
	u64 lastread;
	u64 lastwrite;
	u64 lastsend;
} __attribute__((packed));

struct insp_sklat_event_t {
  char target[TASK_COMM_LEN];
  struct tuple tuple;
  struct skb_meta skb_meta;
  u32 pid;
  u32 cpu;
  u32 direction;
  u64 latency;
};

struct insp_sklat_metric_t {
    u32 netns;
    u32 pid;
    u32 cpu;
    u32 bucket;
    u32 action;
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 10000);
	__type(key, struct sock *);
	__type(value, struct sklat_key_t);
} insp_sklat_entry SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 4096);
	__type(key, struct insp_sklat_metric_t);
	__type(value, u64);
} insp_sklat_metric SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} insp_sklat_events SEC(".maps");

struct insp_sklat_event_t *unused_event __attribute__((unused));

static __always_inline void report_events(struct pt_regs * ctx,struct sock * sk, u64 latency, u32 direction) {
	struct insp_sklat_event_t event = {0};
	set_tuple_sock(sk,&event.tuple);
    set_meta_sock(sk,&event.skb_meta);
    bpf_get_current_comm(&event.target, sizeof(event.target));
    event.pid = bpf_get_current_pid_tgid()>> 32;
    event.cpu = bpf_get_smp_processor_id();
	event.latency = latency;
	event.direction = direction;
	bpf_perf_event_output((struct pt_regs *)ctx, &insp_sklat_events,BPF_F_CURRENT_CPU, &event, sizeof(event));
}

SEC("kprobe/inet_ehash_nolisten")
int sock_create(struct pt_regs *ctx)
{
	struct sock * sk = (struct sock *)PT_REGS_PARM1(ctx);
	struct sklat_key_t ski = {0};
	u64 createat = bpf_ktime_get_ns();
	ski.createat = createat;
	bpf_map_update_elem(&insp_sklat_entry, &sk, &ski, BPF_ANY);
	return 0;
}

SEC("kprobe/sock_def_readable")
int sock_receive(struct pt_regs *ctx)
{
	struct sock * sk = (struct sock *)PT_REGS_PARM1(ctx);
 	struct sklat_key_t * ski;
 	u64 now;
	ski = bpf_map_lookup_elem(&insp_sklat_entry, &sk);
	if (ski) {
	    now =  bpf_ktime_get_ns();
	    ski->lastreceive = now;
	}
	return 0;
}

SEC("kprobe/tcp_cleanup_rbuf")
int sock_read(struct pt_regs *ctx)
{
	struct sock * sk = (struct sock *)PT_REGS_PARM1(ctx);
	int copied = (int)PT_REGS_PARM2(ctx);
 	struct sklat_key_t * ski;
	ski = bpf_map_lookup_elem(&insp_sklat_entry, &sk);
	if (ski) {
        u64 now = bpf_ktime_get_ns();
	    if (ski->lastreceive > 0) {
	        u64 latency;
	        latency = now - ski->lastreceive;
	        if (latency > LAT_THRESH_NS) {
	            struct insp_sklat_metric_t metric = {};
	            metric.pid  = bpf_get_current_pid_tgid()>> 32;
	            metric.cpu = bpf_get_smp_processor_id();
	            metric.netns = get_sock_netns(sk);
                metric.bucket = BUCKET1MS;
	            metric.action = ACTION_READ;
	            if (latency > LAT_THRESH_NS_100MS) {
	                metric.bucket = BUCKET100MS;
					report_events(ctx,sk,latency,ACTION_READ);
	            }
                u64 * mtrv;
	            mtrv = bpf_map_lookup_elem(&insp_sklat_metric, &metric);
                if (mtrv) {
                    __sync_fetch_and_add(mtrv, 1);
                }else{
                    u64 initval = 1;
                    bpf_map_update_elem(&insp_sklat_metric, &metric, &initval, BPF_ANY);
                }
	        }
	    }
        ski->lastread = now;
	}
	return 0;
}

SEC("kprobe/tcp_sendmsg_locked")
int sock_write(struct pt_regs *ctx)
{
	struct sock * sk = (struct sock *)PT_REGS_PARM1(ctx);
 	struct sklat_key_t * ski;
 	u64 now;
	ski = bpf_map_lookup_elem(&insp_sklat_entry, &sk);
	if (ski) {
	    now =  bpf_ktime_get_ns();
 	    ski->lastwrite = now;
	}
	return 0;
}

SEC("kprobe/tcp_write_xmit")
int sock_send(struct pt_regs *ctx)
{
	struct sock * sk = (struct sock *)PT_REGS_PARM1(ctx);
 	struct sklat_key_t * ski;
    u64 now;
	ski = bpf_map_lookup_elem(&insp_sklat_entry, &sk);
	if (ski){
        now = bpf_ktime_get_ns();
	    if (ski->lastwrite > 0) {
	        u64 latency;
	        latency = now - ski->lastwrite;
	        if (latency > LAT_THRESH_NS) {
	            struct insp_sklat_metric_t metric = {};
	            metric.pid  = bpf_get_current_pid_tgid()>> 32;
	            metric.cpu = bpf_get_smp_processor_id();
	            metric.netns = get_sock_netns(sk);
                metric.bucket = BUCKET1MS;
	            metric.action = ACTION_WRITE;
	            if (latency > LAT_THRESH_NS_100MS) {
	                metric.bucket = BUCKET100MS;
					report_events(ctx,sk,latency,ACTION_WRITE);
	            }
                u64 * mtrv;
	            mtrv = bpf_map_lookup_elem(&insp_sklat_metric, &metric);
                if (mtrv) {
                    __sync_fetch_and_add(mtrv, 1);
                }else{
                    u64 initval = 1;
                    bpf_map_update_elem(&insp_sklat_metric, &metric, &initval, BPF_ANY);
                }
	        }
	    }
        ski->lastsend = now;
	}
	return 0;
}


SEC("kprobe/tcp_done")
int sock_destroy(struct pt_regs *ctx)
{
	struct sock * sk = (struct sock *)PT_REGS_PARM1(ctx);
	bpf_map_delete_elem(&insp_sklat_entry, &sk);
	return 0;
}
