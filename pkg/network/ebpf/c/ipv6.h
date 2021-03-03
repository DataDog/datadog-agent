#ifndef __IPV6_H
#define __IPV6_H

/* check if IPs are IPv4 mapped to IPv6 ::ffff:xxxx:xxxx
 * https://tools.ietf.org/html/rfc4291#section-2.5.5
 * the addresses are stored in network byte order so IPv4 adddress is stored
 * in the most significant 32 bits of part saddr_l and daddr_l.
 * Meanwhile the end of the mask is stored in the least significant 32 bits.
 */
static __always_inline bool is_ipv4_mapped_ipv6(__u64 saddr_h, __u64 saddr_l, __u64 daddr_h, __u64 daddr_l) {
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    return ((saddr_h == 0 && ((__u32)saddr_l == 0xFFFF0000)) || (daddr_h == 0 && ((__u32)daddr_l == 0xFFFF0000)));
#elif __BYTE_ORDER__ == __ORDER_BIG_ENDIAN__
    return ((saddr_h == 0 && ((__u32)(saddr_l >> 32) == 0x0000FFFF)) || (daddr_h == 0 && ((__u32)(daddr_l >> 32) == 0x0000FFFF)));
#else
# error "Fix your compiler's __BYTE_ORDER__?!"
#endif
}

static __always_inline void read_in6_addr(u64* addr_h, u64* addr_l, struct in6_addr* in6) {
    bpf_probe_read(addr_h, sizeof(u64), &(in6->in6_u.u6_addr32[0]));
    bpf_probe_read(addr_l, sizeof(u64), &(in6->in6_u.u6_addr32[2]));
}

#endif
