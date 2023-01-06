/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore

#include <vmlinux.h>
#include "bpf_helpers.h"
#include "bpf_core_read.h"
#include "bpf_tracing.h"


struct insp_nflat_in {
    u8 hook;
    u8 nfproto;
    u8 tcp_state;
    u32 padding;
    u64 ts;
};

struct insp_nflat_event_t {

};

