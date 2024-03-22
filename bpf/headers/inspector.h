/* SPDX-License-Identifier: GPL-2.0 WITH Linux-syscall-note */
// +build ignore

#include <bpf/bpf_endian.h>
#include "bpf_core_read.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "vmlinux.h"

#define ETH_P_IP 0x800
#define ETH_P_IPV6 0x86dd

#define PF_INET 2   /* IP protocol family.  */
#define PF_INET6 10 /* IP version 6.  */

#define MAX_STACK_TP 20
#define TASK_COMM_LEN 20
#define KERN_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP)
#define USER_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP | BPF_F_USER_STACK)

#define PERF_MAX_STACK_DEPTH		32

struct flow_tuple_4 {
    unsigned char proto;
    u32 src;
    u32 dst;
    u16 sport;
    u16 dport;
};

union addr {
  unsigned char v6addr[16];
  u32 v4addr;
} __attribute__((packed));

struct skb_meta {
  u32 netns;
  u32 mark;
  u32 ifindex;
  u32 len;
  u32 mtu;
  u32 sk_state;
  u16 protocol;
  u16 pad;
} __attribute__((packed));

struct tuple {
  union addr saddr;
  union addr daddr;
  u16 sport;
  u16 dport;
  u16 l3_proto;
  u8 l4_proto;
  u8 pad;
} __attribute__((packed));

static __always_inline u16 get_sock_protocol(struct sock *sock) {
  u16 protocol = 0;

#ifndef CORE
#if (LINUX_VERSION_CODE < KERNEL_VERSION(5, 6, 0))
  // kernel 4.18-5.5: sk_protocol bit-field: use sk_gso_max_segs field and go
  // back 24 bits to reach sk_protocol field index.
  bpf_probe_read(&protocol, 1, (void *)(&sock->sk_gso_max_segs) - 3);
#else
  // kernel 5.6
  protocol = READ_KERN(sock->sk_protocol);
#endif
#else // CORE
  // commit bf9765145b85 ("sock: Make sk_protocol a 16-bit value")
  struct sock___old *check = NULL;
  if (bpf_core_field_exists(check->__sk_flags_offset)) {
    check = (struct sock___old *)sock;
    bpf_core_read(&protocol, 1, (void *)(&check->sk_gso_max_segs) - 3);
  } else {
    protocol = READ_KERN(sock->sk_protocol);
  }
#endif

  return protocol;
}

static __always_inline u32 get_sock_netns(struct sock *sk) {
  u32 netns;
  netns = BPF_CORE_READ(sk, __sk_common.skc_net.net, ns.inum);
  return netns;
}

static __always_inline u32 get_netns(struct sk_buff *skb) {
  u32 netns;

  struct net_device *dev = BPF_CORE_READ(skb, dev);
  // Get netns id. The code below is equivalent to: netns =
  // dev->nd_net.net->ns.inum
  netns = BPF_CORE_READ(dev, nd_net.net, ns.inum);

  // maybe the skb->dev is not init, for this situation, we can get ns by
  // sk->__sk_common.skc_net.net->ns.inum
  if (netns == 0) {
    struct sock *sk;
    sk = BPF_CORE_READ(skb, sk);
    if (sk != NULL) {
      netns = BPF_CORE_READ(sk, __sk_common.skc_net.net, ns.inum);
    }
  }

  return netns;
}

static __always_inline int set_flow_tuple4(struct __sk_buff *skb, struct flow_tuple_4 *tuple){
	void *data = (void *)(long)skb->data;
	struct ethhdr *eth = data;
	void *data_end = (void *)(long)skb->data_end;
	u16 l4_off = 0;
	const char fmt[] = "source port %d\n";
	//u16 bytes = 0;

	if (data + sizeof(*eth) > data_end)
		return -1;

    if (eth->h_proto == bpf_htons(ETH_P_IP)) {
        struct iphdr *iph = data + sizeof(*eth);

        if (data + sizeof(*eth) + sizeof(*iph) > data_end)
            return -1;

        tuple->src = iph->saddr;
        tuple->dst = iph->daddr;
        tuple->proto = iph->protocol;

        l4_off = sizeof(*eth) + iph->ihl * 4;

        if (iph->protocol == IPPROTO_TCP){
            struct tcphdr *tcph = data + l4_off;

            if (data + l4_off + sizeof(*tcph) > data_end)
                return -1;

            tuple->sport = tcph->source;
            tuple->dport = tcph->dest;
            //bytes = tcph->doff * 4;
        }else if(iph->protocol == IPPROTO_UDP){
            struct udphdr *udph = data + l4_off;
            if(data + l4_off + sizeof(*udph) > data_end)
                return -1;

            tuple->sport = udph->source;
            tuple->dport = udph->dest;
            //bytes = tcph->len;
        }


    } else if (eth->h_proto == bpf_htons(ETH_P_IPV6)) {
        //not supported yet
    }

    return 0;
}

static __always_inline void set_tuple(struct sk_buff *skb, struct tuple *tpl) {
  unsigned char *skb_head = 0;
  u16 l3_off;
  u16 l4_off;
  struct iphdr *ip;
  u8 iphdr_first_byte;
  u8 ip_vsn;
  skb_head = BPF_CORE_READ(skb, head);
  l3_off = BPF_CORE_READ(skb, network_header);
  l4_off = BPF_CORE_READ(skb, transport_header);

  ip = (struct iphdr *)(skb_head + l3_off);
  bpf_probe_read(&iphdr_first_byte, 1, ip);
  ip_vsn = iphdr_first_byte >> 4;

  if (ip_vsn == 4) {
    bpf_probe_read(&tpl->saddr, sizeof(tpl->saddr.v4addr), &ip->saddr);
    bpf_probe_read(&tpl->daddr, sizeof(tpl->daddr.v4addr), &ip->daddr);
    bpf_probe_read(&tpl->l4_proto, 1, &ip->protocol);
    tpl->l3_proto = ETH_P_IP;
  } else if (ip_vsn == 6) {
    struct ipv6hdr *ip6 = (struct ipv6hdr *)ip;
    bpf_probe_read(&tpl->saddr, sizeof(tpl->saddr), &ip6->saddr);
    bpf_probe_read(&tpl->daddr, sizeof(tpl->daddr), &ip6->daddr);
    bpf_probe_read(&tpl->l4_proto, 1, &ip6->nexthdr);
    tpl->l3_proto = ETH_P_IPV6;
  }
  if (tpl->l4_proto == IPPROTO_TCP) {
    struct tcphdr *tcp = (struct tcphdr *)(skb_head + l4_off);
    bpf_probe_read(&tpl->sport, sizeof(tpl->sport), &tcp->source);
    bpf_probe_read(&tpl->dport, sizeof(tpl->dport), &tcp->dest);
  } else if (tpl->l4_proto == IPPROTO_UDP) {
    struct udphdr *udp = (struct udphdr *)(skb_head + l4_off);
    bpf_probe_read(&tpl->sport, sizeof(tpl->sport), &udp->source);
    bpf_probe_read(&tpl->dport, sizeof(tpl->dport), &udp->dest);
  }
}

static __always_inline void set_meta(struct sk_buff *skb,
                                     struct skb_meta *meta) {
  meta->netns = get_netns(skb);
  meta->mark = BPF_CORE_READ(skb, mark);
  meta->len = BPF_CORE_READ(skb, len);
  meta->protocol = BPF_CORE_READ(skb, protocol);
  meta->ifindex = BPF_CORE_READ(skb, dev, ifindex);
  meta->mtu = BPF_CORE_READ(skb, dev, mtu);
}

static __always_inline void set_meta_sock(struct sock *sk,
                                          struct skb_meta *meta) {
  meta->netns = BPF_CORE_READ(sk, __sk_common.skc_net.net, ns.inum);
  meta->protocol = get_sock_protocol(sk);
  // meta->sk_state = BPF_CORE_READ(sk, __sk_common.skc_state);
}

static __always_inline void set_tuple_sock(struct sock *sk, struct tuple *tpl) {
  short unsigned int skc_family;
  skc_family = BPF_CORE_READ(sk, __sk_common.skc_family);
  if (skc_family == PF_INET6) {
    // TODO: add v6 sock support
    tpl->l3_proto = ETH_P_IPV6;
  } else {
    bpf_probe_read(&tpl->saddr, sizeof(tpl->saddr.v4addr),
                   &sk->__sk_common.skc_rcv_saddr);
    bpf_probe_read(&tpl->daddr, sizeof(tpl->daddr.v4addr),
                   &sk->__sk_common.skc_daddr);
    tpl->l3_proto = ETH_P_IP;
  }

  tpl->sport = BPF_CORE_READ(sk, __sk_common.skc_num);
  tpl->dport = BPF_CORE_READ(sk, __sk_common.skc_dport);
  tpl->l4_proto = get_sock_protocol(sk);
  ;
}
