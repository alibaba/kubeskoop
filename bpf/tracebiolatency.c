/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore

#include <vmlinux.h>
#include <bpf_helpers.h>
#include <bpf_tracing.h>
#include <bpf_core_read.h>
#include <inspector.h>


struct insp_biolat_metric_t  {
    u32 pid;
    u32 bucket;
};

struct insp_biolat_event_t {
    char target[TASK_COMM_LEN];
    char disk[TASK_COMM_LEN];
    u32 pid;
    u64 latency;
};

struct insp_biolat_entry_t  {
    char target[TASK_COMM_LEN];
    char disk[TASK_COMM_LEN];
    u32 pid;
    u64 start;
    u64 latency;
}__attribute__((packed));

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, struct request *);
	__type(value, struct insp_biolat_entry_t );
	__uint(max_entries, 10000);
} insp_biolat_metric SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, struct request *);
	__type(value, struct insp_biolat_entry_t );
	__uint(max_entries, 10000);
} insp_biolat_entry SEC(".maps");


struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
	__uint(key_size, sizeof(u32));
	__uint(value_size, sizeof(u32));
} insp_biolat_evts SEC(".maps");


struct insp_biolat_event_t *unused_event __attribute__((unused));

SEC("kprobe/blk_account_io_start")
int biolat_start(struct pt_regs *ctx)
{
    struct request * rq = (struct request *)PT_REGS_PARM1(ctx);
    struct insp_biolat_entry_t et = {0};
	et.pid  = bpf_get_current_pid_tgid()>> 32;
	et.start = bpf_ktime_get_ns();
	// bpf_printk("now %llu\n",et.start);
	// void * __tmp = (void *)rq->rq_disk->disk_name;
    // bpf_probe_read(&et.disk, sizeof(et.disk), __tmp);
    bpf_get_current_comm(&et.target, sizeof(et.target));
    bpf_map_update_elem(&insp_biolat_entry,&rq,&et,0);
    return 0;
}

SEC("kprobe/blk_account_io_done")
int biolat_finish(struct pt_regs *ctx)
{
   struct request * rq = (struct request *)PT_REGS_PARM1(ctx);
   struct insp_biolat_entry_t * biot = bpf_map_lookup_elem(&insp_biolat_entry, &rq);
   if (biot){
       u64 now = bpf_ktime_get_ns();
       u64 latency;
	   latency = now - biot->start;
	   if (latency > 10000000){
	   	   bpf_printk("now %llu start %llu latency %llu\n",now,biot->start,latency);
           struct insp_biolat_event_t event = {0};
           event.latency = latency;
           event.pid = biot->pid;
           bpf_probe_read(&event.target,sizeof(event.target),&biot->target);
           bpf_perf_event_output(ctx, &insp_biolat_evts, BPF_F_CURRENT_CPU, &event, sizeof(event));
	   }
       bpf_map_delete_elem(&insp_biolat_entry, &rq);
   }

   return 0;
}



char LICENSE[] SEC("license") = "Dual BSD/GPL";
