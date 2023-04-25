#ifndef __IPV6_H
#define __IPV6_H

#include "bpf_core_read.h"
#include "bpf_telemetry.h"

#include "defs.h"

#ifndef COMPILE_CORE
#include <uapi/linux/in6.h>
#endif

/* check if IPs are IPv4 mapped to IPv6 ::ffff:xxxx:xxxx
 * https://tools.ietf.org/html/rfc4291#section-2.5.5
 * the addresses are stored in network byte order so IPv4 adddress is stored
 * in the most significant 32 bits of part saddr_l and daddr_l.
 * Meanwhile the end of the mask is stored in the least significant 32 bits.
 */
// On older kernels, clang can generate Wunused-function warnings on static inline functions defined in
// header files, even if they are later used in source files. __maybe_unused prevents that issue
__maybe_unused static __always_inline bool is_ipv4_mapped_ipv6(__u64 saddr_h, __u64 saddr_l, __u64 daddr_h, __u64 daddr_l) {
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    return ((saddr_h == 0 && ((__u32)saddr_l == 0xFFFF0000)) || (daddr_h == 0 && ((__u32)daddr_l == 0xFFFF0000)));
#elif __BYTE_ORDER__ == __ORDER_BIG_ENDIAN__
    return ((saddr_h == 0 && ((__u32)(saddr_l >> 32) == 0x0000FFFF)) || (daddr_h == 0 && ((__u32)(daddr_l >> 32) == 0x0000FFFF)));
#else
#error "Fix your compiler's __BYTE_ORDER__?!"
#endif
}

static __always_inline void read_in6_addr(u64 *addr_h, u64 *addr_l, const struct in6_addr *in6) {
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(addr_h, sizeof(u64), (void *)&(in6->in6_u.u6_addr32[0]));
    bpf_probe_read_kernel_with_telemetry(addr_l, sizeof(u64), (void *)&(in6->in6_u.u6_addr32[2]));
#else
    BPF_CORE_READ_INTO(addr_h, in6, in6_u.u6_addr32[0]);
    BPF_CORE_READ_INTO(addr_l, in6, in6_u.u6_addr32[2]);
#endif
}

static __maybe_unused __always_inline bool is_ipv6_enabled() {
#ifdef COMPILE_RUNTIME
#ifdef FEATURE_IPV6_ENABLED
    return true;
#else
    return false;
#endif
#else
    __u64 val = 0;
    LOAD_CONSTANT("ipv6_enabled", val);
    return val == ENABLED;
#endif
}

#endif
