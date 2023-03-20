#ifndef __IP_H
#define __IP_H

#include "ktypes.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "bpf_endian.h"

#include "tracer.h"

#ifdef COMPILE_CORE
#define AF_INET 2 /* Internet IP Protocol */
#define AF_INET6 10 /* IP version 6 */

// from uapi/linux/if_ether.h
#define ETH_HLEN 14 /* Total octets in header. */
#define ETH_P_IP 0x0800 /* Internet Protocol packet */
#define ETH_P_IPV6 0x86DD /* IPv6 over bluebook */
#else
#include <uapi/linux/if_ether.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/udp.h>
#endif

// from uapi/linux/tcp.h
struct __tcphdr {
	__be16	source;
	__be16	dest;
	__be32	seq;
	__be32	ack_seq;
	__u16	res1:4,
		doff:4,
		fin:1,
		syn:1,
		rst:1,
		psh:1,
		ack:1,
		urg:1,
		ece:1,
		cwr:1;
	__be16	window;
	__sum16	check;
	__be16	urg_ptr;
};

// from uapi/linux/in.h
#define __IPPROTO_TCP 6
#define __IPPROTO_UDP 17

// TODO: these are mostly hacky placeholders until we decide on what is the best
// approach to work around the eBPF bug described here:
// https://github.com/torvalds/linux/commit/e6a18d36118bea3bf497c9df4d9988b6df120689
// We should consider using something like `asm volatile("":::"r1")` so this patch
// can also benefit hosts running kernel 4.4
__maybe_unused static __always_inline u64 __load_word(void *ptr, u32 offset) {
#ifdef COMPILE_PREBUILT
    return load_word(ptr, offset);
#else
    if (bpf_helper_exists(BPF_FUNC_skb_load_bytes)) {
        u32 res = 0;
        bpf_skb_load_bytes(ptr, offset, &res, sizeof(res));
        return bpf_htonl(res);
    }
    return load_word(ptr, offset);
#endif
}

__maybe_unused static __always_inline u64 __load_half(void *ptr, u32 offset) {
#ifdef COMPILE_PREBUILT
    return load_half(ptr, offset);
#else
    if (bpf_helper_exists(BPF_FUNC_skb_load_bytes)) {
        u16 res = 0;
        bpf_skb_load_bytes(ptr, offset, &res, sizeof(res));
        return bpf_htons(res);
    }
    return load_half(ptr, offset);
#endif
}

__maybe_unused static __always_inline u64 __load_byte(void *ptr, u32 offset) {
#ifdef COMPILE_PREBUILT
    return load_byte(ptr, offset);
#else
    if (bpf_helper_exists(BPF_FUNC_skb_load_bytes)) {
        u8 res = 0;
        bpf_skb_load_bytes(ptr, offset, &res, sizeof(res));
        return res;
    }
    return load_byte(ptr, offset);
#endif
}

static __always_inline void read_ipv6_skb(struct __sk_buff *skb, __u64 off, __u64 *addr_l, __u64 *addr_h) {
    *addr_h |= (__u64)__load_word(skb, off) << 32;
    *addr_h |= (__u64)__load_word(skb, off + 4);
    *addr_h = bpf_ntohll(*addr_h);

    *addr_l |= (__u64)__load_word(skb, off + 8) << 32;
    *addr_l |= (__u64)__load_word(skb, off + 12);
    *addr_l = bpf_ntohll(*addr_l);
}

static __always_inline void read_ipv4_skb(struct __sk_buff *skb, __u64 off, __u64 *addr) {
    *addr = __load_word(skb, off);
    *addr = bpf_ntohll(*addr) >> 32;
}

// On older kernels, clang can generate Wunused-function warnings on static inline functions defined in
// header files, even if they are later used in source files. __maybe_unused prevents that issue
__maybe_unused static __always_inline __u64 read_conn_tuple_skb(struct __sk_buff *skb, skb_info_t *info, conn_tuple_t *tup) {
    bpf_memset(info, 0, sizeof(skb_info_t));
    info->data_off = ETH_HLEN;

    __u16 l3_proto = __load_half(skb, offsetof(struct ethhdr, h_proto));
    __u8 l4_proto = 0;
    switch (l3_proto) {
    case ETH_P_IP:
    {
        __u8 ipv4_hdr_len = (__load_byte(skb, info->data_off) & 0x0f) << 2;
        if (ipv4_hdr_len < sizeof(struct iphdr)) {
            return 0;
        }
        l4_proto = __load_byte(skb, info->data_off + offsetof(struct iphdr, protocol));
        tup->metadata |= CONN_V4;
        read_ipv4_skb(skb, info->data_off + offsetof(struct iphdr, saddr), &tup->saddr_l);
        read_ipv4_skb(skb, info->data_off + offsetof(struct iphdr, daddr), &tup->daddr_l);
        info->data_off += ipv4_hdr_len;
        break;
    }
    case ETH_P_IPV6:
        l4_proto = __load_byte(skb, info->data_off + offsetof(struct ipv6hdr, nexthdr));
        tup->metadata |= CONN_V6;
        read_ipv6_skb(skb, info->data_off + offsetof(struct ipv6hdr, saddr), &tup->saddr_l, &tup->saddr_h);
        read_ipv6_skb(skb, info->data_off + offsetof(struct ipv6hdr, daddr), &tup->daddr_l, &tup->daddr_h);
        info->data_off += sizeof(struct ipv6hdr);
        break;
    default:
        return 0;
    }

    switch (l4_proto) {
    case __IPPROTO_UDP:
        tup->metadata |= CONN_TYPE_UDP;
        tup->sport = __load_half(skb, info->data_off + offsetof(struct udphdr, source));
        tup->dport = __load_half(skb, info->data_off + offsetof(struct udphdr, dest));
        info->data_off += sizeof(struct udphdr);
        break;
    case __IPPROTO_TCP:
        tup->metadata |= CONN_TYPE_TCP;
        tup->sport = __load_half(skb, info->data_off + offsetof(struct __tcphdr, source));
        tup->dport = __load_half(skb, info->data_off + offsetof(struct __tcphdr, dest));

        info->tcp_seq = __load_word(skb, info->data_off + offsetof(struct __tcphdr, seq));
        info->tcp_flags = __load_byte(skb, info->data_off + TCP_FLAGS_OFFSET);
        // TODO: Improve readability and explain the bit twiddling below
        info->data_off += ((__load_byte(skb, info->data_off + offsetof(struct __tcphdr, ack_seq) + 4) & 0xF0) >> 4) * 4;
        break;
    default:
        return 0;
    }

    if ((skb->len - info->data_off) < 0) {
        return 0;
    }

    return 1;
}

__maybe_unused static __always_inline bool is_equal(conn_tuple_t *t, conn_tuple_t *t2) {
    bool match = !bpf_memcmp(t, t2, sizeof(conn_tuple_t));
    return match;
}

// On older kernels, clang can generate Wunused-function warnings on static inline functions defined in
// header files, even if they are later used in source files. __maybe_unused prevents that issue
__maybe_unused static __always_inline void flip_tuple(conn_tuple_t *t) {
    // TODO: we can probably replace this by swap operations
    __u16 tmp_port = t->sport;
    t->sport = t->dport;
    t->dport = tmp_port;

    __u64 tmp_ip_part = t->saddr_l;
    t->saddr_l = t->daddr_l;
    t->daddr_l = tmp_ip_part;

    tmp_ip_part = t->saddr_h;
    t->saddr_h = t->daddr_h;
    t->daddr_h = tmp_ip_part;
}

// On older kernels, clang can generate Wunused-function warnings on static inline functions defined in
// header files, even if they are later used in source files. __maybe_unused prevents that issue
__maybe_unused static __always_inline void print_ip(u64 ip_h, u64 ip_l, u16 port, u32 metadata) {
// support for %pI4 and %pI6 added in https://github.com/torvalds/linux/commit/d9c9e4db186ab4d81f84e6f22b225d333b9424e3
#if defined(COMPILE_RUNTIME) && defined(LINUX_VERSION_CODE) && LINUX_VERSION_CODE >= KERNEL_VERSION(5, 13, 0)
    if (metadata & CONN_V6) {
        struct in6_addr addr;
        addr.in6_u.u6_addr32[0] = ip_h & 0xFFFFFFFF;
        addr.in6_u.u6_addr32[1] = (ip_h >> 32) & 0xFFFFFFFF;
        addr.in6_u.u6_addr32[2] = ip_l & 0xFFFFFFFF;
        addr.in6_u.u6_addr32[3] = (ip_l >> 32) & 0xFFFFFFFF;
        log_debug("v6 %pI6:%u\n", &addr, port);
    } else {
        log_debug("v4 %pI4:%u\n", &ip_l, port);
    }
#else
    if (metadata & CONN_V6) {
        log_debug("v6 %llx%llx:%u\n", bpf_ntohll(ip_h), bpf_ntohll(ip_l), port);
    } else {
        log_debug("v4 %x:%u\n", bpf_ntohl((u32)ip_l), port);
    }
#endif
}

#endif
