#ifndef __IP_H
#define __IP_H

#include "ktypes.h"
#include "defs.h"
#include "bpf_builtins.h"
#include "bpf_core_read.h"
#include "bpf_endian.h"
#include "bpf_tracing.h"
#include "compiler.h"

#include "conn_tuple.h"

#define TCPHDR_FIN 0x01
#define TCPHDR_RST 0x04
#define TCPHDR_ACK 0x10

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

// skb_info_t embeds a conn_tuple_t extracted from the skb object as well as
// some ancillary data such as the data offset (the byte offset pointing to
// where the application payload begins) and the TCP flags if applicable.
// This struct is populated by calling `read_conn_tuple_skb` from a program type
// that manipulates a `__sk_buff` object.
typedef struct {
    __u32 data_off;
    __u32 data_end;
    __u32 tcp_seq;
    __u8 tcp_flags;
} skb_info_t;

// Parses the headers with one load per field. Prebuilt always uses this; runtime-compiled builds use it only when
// bpf_skb_load_bytes is unavailable.
// On older kernels, clang can generate Wunused-function warnings on static inline functions defined in
// header files, even if they are later used in source files. __maybe_unused prevents that issue
__maybe_unused static __always_inline __u64 read_conn_tuple_skb_fields(struct __sk_buff *skb, skb_info_t *info, conn_tuple_t *tup) {
    bpf_memset(info, 0, sizeof(skb_info_t));
    info->data_off = ETH_HLEN;

    __u16 l3_proto = __load_half(skb, offsetof(struct ethhdr, h_proto));
    info->data_end = ETH_HLEN;
    __u8 l4_proto = 0;
    switch (l3_proto) {
    case ETH_P_IP:
    {
        __u8 ipv4_hdr_len = (__load_byte(skb, info->data_off) & 0x0f) << 2;
        info->data_end += __load_half(skb, info->data_off + offsetof(struct iphdr, tot_len));
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
        info->data_end += sizeof(struct ipv6hdr) + __load_half(skb, info->data_off + offsetof(struct ipv6hdr, payload_len));
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

    if ((info->data_end - info->data_off) < 0) {
        return 0;
    }

    return 1;
}

#ifndef COMPILE_PREBUILT
// Header bytes are loaded starting at this frame offset so every multi-byte field read below sits at a stack
// offset aligned to its size, which the verifier requires.
#define SKB_HDR_OFF 6

// Plain header offsets rather than offsetof on kernel types: on CO-RE those are relocated register values, and
// older verifiers reject a load from the stack pointer plus a register.
#define SKB_ETH_PROTO_OFF 12
#define SKB_IPV4_HLEN 20
#define SKB_IPV4_TOT_LEN_OFF 2
#define SKB_IPV4_PROTO_OFF 9
#define SKB_IPV4_SADDR_OFF 12
#define SKB_IPV4_DADDR_OFF 16
#define SKB_IPV6_HLEN 40
#define SKB_IPV6_PAYLOAD_LEN_OFF 4
#define SKB_IPV6_NEXTHDR_OFF 6
#define SKB_IPV6_SADDR_OFF 8
#define SKB_IPV6_DADDR_OFF 24
#define SKB_UDP_HLEN 8

// Through the TCP flags of an IPv4 packet without options.
#define SKB_HDR_V4_LEN (ETH_HLEN + SKB_IPV4_HLEN + TCP_FLAGS_OFFSET + 1 - SKB_HDR_OFF)
// Through the TCP flags of an IPv6 packet.
#define SKB_HDR_V6_LEN (ETH_HLEN + SKB_IPV6_HLEN + TCP_FLAGS_OFFSET + 1 - SKB_HDR_OFF)

typedef union {
    __u8 b[64];
    __u16 h[32];
    __u32 w[16];
    __u64 d[8];
} skb_hdr_buf_t;

// Accessors take frame offsets and return the same values as __load_byte/__load_half/__load_word.
#define SKB_HDR_BYTE(hdr, off) ((__u64)(hdr)->b[(off) - SKB_HDR_OFF])
#define SKB_HDR_HALF(hdr, off) ((__u64)bpf_ntohs((hdr)->h[((off) - SKB_HDR_OFF) / 2]))
#define SKB_HDR_WORD(hdr, off) ((__u64)bpf_ntohl((hdr)->w[((off) - SKB_HDR_OFF) / 4]))
// Equals __load_word(off) << 32 | __load_word(off + 4).
#define SKB_HDR_DWORD(hdr, off) bpf_ntohll((hdr)->d[((off) - SKB_HDR_OFF) / 8])

// Loads one field into its slot in hdr. A field past the end of the frame reads as zero, as in the per-field
// parse, since bpf_skb_load_bytes zeroes the destination when the load fails.
#define SKB_HDR_FIELD(skb, hdr, off, size) bpf_skb_load_bytes(skb, off, &(hdr)->b[(off) - SKB_HDR_OFF], size)

// Loads the L4 header at frame offset off into hdr at slot, the frame offset it has without IP options. Falls back
// to the fields the parse reads: ports, seq, doff, flags. A macro so each destination stays the frame pointer plus a
// constant: verifiers before 4.14 reject adding a constant to a stack pointer held in another register.
#define SKB_HDR_L4(skb, hdr, slot, off)                                                                   \
    do {                                                                                                  \
        if (bpf_skb_load_bytes(skb, off, &(hdr)->b[(slot) - SKB_HDR_OFF], TCP_FLAGS_OFFSET + 1) < 0) {   \
            bpf_skb_load_bytes(skb, off, &(hdr)->b[(slot) - SKB_HDR_OFF], 2);                             \
            bpf_skb_load_bytes(skb, (off) + 2, &(hdr)->b[(slot) + 2 - SKB_HDR_OFF], 2);                   \
            bpf_skb_load_bytes(skb, (off) + 4, &(hdr)->b[(slot) + 4 - SKB_HDR_OFF], 4);                   \
            bpf_skb_load_bytes(skb, (off) + 12, &(hdr)->b[(slot) + 12 - SKB_HDR_OFF], 1);                 \
            bpf_skb_load_bytes(skb, (off) + TCP_FLAGS_OFFSET, &(hdr)->b[(slot) + TCP_FLAGS_OFFSET - SKB_HDR_OFF], 1); \
        }                                                                                                 \
    } while (0)

// Same result as read_conn_tuple_skb_fields, usually from one or two bpf_skb_load_bytes calls instead of one per
// field. When a bulk load runs past the end of the frame, the fields it covers are loaded one by one into the same
// buffer, so the parse below sees what the per-field parse would. Returns -1 without touching info or tup only
// when bpf_skb_load_bytes is unavailable.
static __always_inline int read_conn_tuple_skb_bulk(struct __sk_buff *skb, skb_info_t *info, conn_tuple_t *tup) {
    if (!bpf_helper_exists(BPF_FUNC_skb_load_bytes)) {
        return -1;
    }

    skb_hdr_buf_t hdr;
    const bool full = bpf_skb_load_bytes(skb, SKB_HDR_OFF, &hdr, SKB_HDR_V4_LEN) == 0;
    if (!full) {
        SKB_HDR_FIELD(skb, &hdr, SKB_ETH_PROTO_OFF, 2);
    }

    bpf_memset(info, 0, sizeof(skb_info_t));
    info->data_off = ETH_HLEN;
    info->data_end = ETH_HLEN;

    const __u32 ip = ETH_HLEN;
    const __u8 *l4;
    __u8 l4_proto;
    switch (SKB_HDR_HALF(&hdr, SKB_ETH_PROTO_OFF)) {
    case ETH_P_IP:
    {
        if (!full) {
            SKB_HDR_FIELD(skb, &hdr, ip, 1);
            SKB_HDR_FIELD(skb, &hdr, ip + SKB_IPV4_TOT_LEN_OFF, 2);
        }
        __u8 ipv4_hdr_len = (SKB_HDR_BYTE(&hdr, ip) & 0x0f) << 2;
        info->data_end += SKB_HDR_HALF(&hdr, ip + SKB_IPV4_TOT_LEN_OFF);
        if (ipv4_hdr_len < SKB_IPV4_HLEN) {
            return 0;
        }
        if (!full) {
            SKB_HDR_FIELD(skb, &hdr, ip + SKB_IPV4_PROTO_OFF, 1);
            SKB_HDR_FIELD(skb, &hdr, ip + SKB_IPV4_SADDR_OFF, 4);
            SKB_HDR_FIELD(skb, &hdr, ip + SKB_IPV4_DADDR_OFF, 4);
        }
        l4_proto = SKB_HDR_BYTE(&hdr, ip + SKB_IPV4_PROTO_OFF);
        tup->metadata |= CONN_V4;
        tup->saddr_l = bpf_ntohll(SKB_HDR_WORD(&hdr, ip + SKB_IPV4_SADDR_OFF)) >> 32;
        tup->daddr_l = bpf_ntohll(SKB_HDR_WORD(&hdr, ip + SKB_IPV4_DADDR_OFF)) >> 32;
        info->data_off += ipv4_hdr_len;
        // With IP options, the L4 header is loaded over the option bytes, which aren't used.
        if (!full || ipv4_hdr_len != SKB_IPV4_HLEN) {
            SKB_HDR_L4(skb, &hdr, ip + SKB_IPV4_HLEN, info->data_off);
        }
        l4 = &hdr.b[ip + SKB_IPV4_HLEN - SKB_HDR_OFF];
        break;
    }
    case ETH_P_IPV6:
    {
        const bool loaded = full && bpf_skb_load_bytes(skb, SKB_HDR_OFF + SKB_HDR_V4_LEN, &hdr.b[SKB_HDR_V4_LEN], SKB_HDR_V6_LEN - SKB_HDR_V4_LEN) == 0;
        const __u32 saddr = ip + SKB_IPV6_SADDR_OFF;
        const __u32 daddr = ip + SKB_IPV6_DADDR_OFF;
        if (!loaded) {
            SKB_HDR_FIELD(skb, &hdr, ip + SKB_IPV6_PAYLOAD_LEN_OFF, 2);
            SKB_HDR_FIELD(skb, &hdr, ip + SKB_IPV6_NEXTHDR_OFF, 1);
            SKB_HDR_FIELD(skb, &hdr, saddr, 4);
            SKB_HDR_FIELD(skb, &hdr, saddr + 4, 4);
            SKB_HDR_FIELD(skb, &hdr, saddr + 8, 4);
            SKB_HDR_FIELD(skb, &hdr, saddr + 12, 4);
            SKB_HDR_FIELD(skb, &hdr, daddr, 4);
            SKB_HDR_FIELD(skb, &hdr, daddr + 4, 4);
            SKB_HDR_FIELD(skb, &hdr, daddr + 8, 4);
            SKB_HDR_FIELD(skb, &hdr, daddr + 12, 4);
            SKB_HDR_L4(skb, &hdr, ip + SKB_IPV6_HLEN, ip + SKB_IPV6_HLEN);
        }
        info->data_end += SKB_IPV6_HLEN + SKB_HDR_HALF(&hdr, ip + SKB_IPV6_PAYLOAD_LEN_OFF);
        l4_proto = SKB_HDR_BYTE(&hdr, ip + SKB_IPV6_NEXTHDR_OFF);
        tup->metadata |= CONN_V6;
        tup->saddr_h = bpf_ntohll(tup->saddr_h | SKB_HDR_DWORD(&hdr, saddr));
        tup->saddr_l = bpf_ntohll(tup->saddr_l | SKB_HDR_DWORD(&hdr, saddr + 8));
        tup->daddr_h = bpf_ntohll(tup->daddr_h | SKB_HDR_DWORD(&hdr, daddr));
        tup->daddr_l = bpf_ntohll(tup->daddr_l | SKB_HDR_DWORD(&hdr, daddr + 8));
        info->data_off += SKB_IPV6_HLEN;
        l4 = &hdr.b[ip + SKB_IPV6_HLEN - SKB_HDR_OFF];
        break;
    }
    default:
        return 0;
    }

    // l4 is at a fixed, 4-byte aligned stack offset on each path. UDP's ports share the TCP layout.
    const struct __tcphdr *th = (const struct __tcphdr *)l4;

    switch (l4_proto) {
    case __IPPROTO_UDP:
        tup->metadata |= CONN_TYPE_UDP;
        tup->sport = bpf_ntohs(th->source);
        tup->dport = bpf_ntohs(th->dest);
        info->data_off += SKB_UDP_HLEN;
        return 1;
    case __IPPROTO_TCP:
        tup->metadata |= CONN_TYPE_TCP;
        tup->sport = bpf_ntohs(th->source);
        tup->dport = bpf_ntohs(th->dest);
        info->tcp_seq = bpf_ntohl(th->seq);
        info->tcp_flags = l4[TCP_FLAGS_OFFSET];
        info->data_off += ((l4[offsetof(struct __tcphdr, ack_seq) + 4] & 0xF0) >> 4) * 4;
        return 1;
    default:
        return 0;
    }
}
#endif // !COMPILE_PREBUILT

// On older kernels, clang can generate Wunused-function warnings on static inline functions defined in
// header files, even if they are later used in source files. __maybe_unused prevents that issue
__maybe_unused static __always_inline __u64 read_conn_tuple_skb(struct __sk_buff *skb, skb_info_t *info, conn_tuple_t *tup) {
#ifndef COMPILE_PREBUILT
    int ret = read_conn_tuple_skb_bulk(skb, info, tup);
    if (ret >= 0) {
        return ret;
    }
#endif
#ifdef COMPILE_CORE
    // Socket filters get bpf_skb_load_bytes in 4.11, below every kernel these programs load on, so the per-field
    // parse would be dead code. Kernels before 5.1 keep dead code, and it would count against optmem_max.
    return 0;
#else
    return read_conn_tuple_skb_fields(skb, info, tup);
#endif
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
        log_debug("v6 %pI6:%u", &addr, port);
    } else {
        log_debug("v4 %pI4:%u", &ip_l, port);
    }
#else
    if (metadata & CONN_V6) {
        log_debug("v6 %llx%llx:%u", bpf_ntohll(ip_h), bpf_ntohll(ip_l), port);
    } else {
        log_debug("v4 %x:%u", bpf_ntohl((u32)ip_l), port);
    }
#endif
}

static __maybe_unused __always_inline bool is_tcpv4_enabled() {
#ifdef COMPILE_RUNTIME
#ifdef FEATURE_TCPV4_ENABLED
    return true;
#else
    return false;
#endif
#else
    __u64 val = 0;
    LOAD_CONSTANT("tcpv4_enabled", val);
    return val == ENABLED;
#endif
}

static __maybe_unused __always_inline bool is_udpv4_enabled() {
#ifdef COMPILE_RUNTIME
#ifdef FEATURE_UDPV4_ENABLED
    return true;
#else
    return false;
#endif
#else
    __u64 val = 0;
    LOAD_CONSTANT("udpv4_enabled", val);
    return val == ENABLED;
#endif
}

#endif
