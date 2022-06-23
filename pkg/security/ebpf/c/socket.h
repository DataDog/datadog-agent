#ifndef _SOCKET_H_
#define _SOCKET_H_

#define DECLARE_EQUAL_TO_SUFFIXED(suffix, s) static inline int equal_to_##suffix(char *str) { \
        char s1[sizeof(#s)];                                            \
        bpf_probe_read(&s1, sizeof(s1), str);                           \
        char s2[] = #s;                                                 \
        for (int i = 0; i < sizeof(s1); ++i)                            \
            if (s2[i] != s1[i])                                         \
                return 0;                                               \
        return 1;                                                       \
    }                                                                   \

#define DECLARE_EQUAL_TO(s) \
    DECLARE_EQUAL_TO_SUFFIXED(s, s)

#define IS_EQUAL_TO(str, s) equal_to_##s(str)

DECLARE_EQUAL_TO(TCP)
DECLARE_EQUAL_TO(TCPv6)

DECLARE_EQUAL_TO(UDP)
DECLARE_EQUAL_TO(UDPv6)

DECLARE_EQUAL_TO(PING)
DECLARE_EQUAL_TO(PINGv6)

DECLARE_EQUAL_TO(RAW)
DECLARE_EQUAL_TO(RAWv6)

DECLARE_EQUAL_TO(SCTP)
DECLARE_EQUAL_TO(SCTPv6)

DECLARE_EQUAL_TO_SUFFIXED(UDPLite, UDP-Lite)
DECLARE_EQUAL_TO(UDPLITEv6)

DECLARE_EQUAL_TO(DCCP)
DECLARE_EQUAL_TO(DCCPv6)

__attribute__((always_inline)) u8 get_ipproto_id(char* proto) {
    if (IS_EQUAL_TO(proto, TCP) || IS_EQUAL_TO(proto, TCPv6))
        return IPPROTO_TCP;
    else if (IS_EQUAL_TO(proto, UDP) || IS_EQUAL_TO(proto, UDPv6))
        return IPPROTO_UDP;
    else if (IS_EQUAL_TO(proto, PING) || IS_EQUAL_TO(proto, PINGv6))
        return IPPROTO_ICMP;
    else if (IS_EQUAL_TO(proto, RAW) || IS_EQUAL_TO(proto, RAWv6))
        return IPPROTO_IP;
    else if (IS_EQUAL_TO(proto, UDPLite) || IS_EQUAL_TO(proto, UDPLITEv6))
        return IPPROTO_UDPLITE;
    else if (IS_EQUAL_TO(proto, SCTP) || IS_EQUAL_TO(proto, SCTPv6))
        return IPPROTO_SCTP;
    else if (IS_EQUAL_TO(proto, DCCP) || IS_EQUAL_TO(proto, DCCPv6))
        return IPPROTO_DCCP;
    return IPPROTO_IP;
}

__attribute__((always_inline)) u8 get_protocol_from_proto(struct proto *skc_prot) {
    u64 proto_name_offset;
    LOAD_CONSTANT("proto_name_offset", proto_name_offset);

    char name [32] = {};
    bpf_probe_read(&name, sizeof(name), (void *)skc_prot + proto_name_offset);

    return get_ipproto_id(name);
}

__attribute__((always_inline)) u8 get_protocol_from_sock(struct sock *sk) {
    u64 sock_common_skc_prot_offset;
    LOAD_CONSTANT("sock_common_skc_prot_offset", sock_common_skc_prot_offset);

    struct sock_common *common = (void *)sk;
    struct proto *skc_prot = NULL;
    bpf_probe_read(&skc_prot, sizeof(skc_prot), (void *)common + sock_common_skc_prot_offset);
    return get_protocol_from_proto(skc_prot);
}

#endif /* _SOCKET_H_ */
