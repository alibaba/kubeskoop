package tracetasklatency

/*

 Tracepoint for accounting wait time (time the task is runnable
 but not actually running due to scheduler contention).

 DEFINE_EVENT(sched_stat_template, sched_stat_wait,
	TP_PROTO(struct task_struct *tsk, u64 delay),
	TP_ARGS(tsk, delay));


Tracepoint for accounting sleep time (time the task is not runnable,
including iowait, see below).

DEFINE_EVENT(sched_stat_template, sched_stat_sleep,
	TP_PROTO(struct task_struct *tsk, u64 delay),
	TP_ARGS(tsk, delay));


 Tracepoint for accounting iowait time (time the task is not runnable
 due to waiting on IO to complete).

DEFINE_EVENT(sched_stat_template, sched_stat_iowait,
	TP_PROTO(struct task_struct *tsk, u64 delay),
	TP_ARGS(tsk, delay));
*/
