#ifndef _CONSTANTS_OFFSETS_NETWORK_H_
#define _CONSTANTS_OFFSETS_NETWORK_H_

#include "constants/macros.h"

__attribute__((always_inline)) u16 get_family_from_sock_common(struct sock_common *sk) {
    u64 sock_common_skc_family_offset;
    LOAD_CONSTANT("sock_common_skc_family_offset", sock_common_skc_family_offset);

    u16 family;
    bpf_probe_read(&family, sizeof(family), (void *)sk + sock_common_skc_family_offset);
    return family;
}

__attribute__((always_inline)) u16 get_skc_num_from_sock_common(struct sock_common *sk) {
    u64 sock_common_skc_num_offset;
    LOAD_CONSTANT("sock_common_skc_num_offset", sock_common_skc_num_offset);

    u16 skc_num;
    bpf_probe_read(&skc_num, sizeof(skc_num), (void *)sk + sock_common_skc_num_offset);
    return htons(skc_num);
}
__attribute__((always_inline)) u16 get_protocol_from_sock(struct sock *sk) {
    u64 sock_sk_protocol_offset;
    LOAD_CONSTANT("sock_sk_protocol_offset", sock_sk_protocol_offset);
    u16 protocol = 0;
    if ((void *)sk + sock_sk_protocol_offset > 0 && sock_sk_protocol_offset + sizeof(protocol) < sizeof(struct sock)) {
        bpf_probe_read(&protocol, sizeof(protocol), (void *)sk + sock_sk_protocol_offset);
        }
        return protocol & 0xff;
    return 0; // Default value if protocol cannot be determined

}

__attribute__((always_inline)) struct sock* get_sock_from_socket(struct socket *socket) {
    u64 socket_sock_offset;
    LOAD_CONSTANT("socket_sock_offset", socket_sock_offset);

    struct sock *sk = NULL;
    bpf_probe_read(&sk, sizeof(sk), (void *)socket + socket_sock_offset);
    return sk;
}

__attribute__((always_inline)) u64 get_flowi4_saddr_offset() {
    u64 flowi4_saddr_offset;
    LOAD_CONSTANT("flowi4_saddr_offset", flowi4_saddr_offset);
    return flowi4_saddr_offset;
}

// TODO: needed for l4_protocol resolution, see network/flow.h
__attribute__((always_inline)) u64 get_flowi4_proto_offset() {
    u64 flowi4_proto_offset;
    LOAD_CONSTANT("flowi4_proto_offset", flowi4_proto_offset);
    return flowi4_proto_offset;
}

__attribute__((always_inline)) u64 get_flowi6_proto_offset() {
    u64 flowi6_proto_offset;
    LOAD_CONSTANT("flowi6_proto_offset", flowi6_proto_offset);
    return flowi6_proto_offset;
}

__attribute__((always_inline)) u64 get_flowi4_uli_offset() {
    u64 flowi4_uli_offset;
    LOAD_CONSTANT("flowi4_uli_offset", flowi4_uli_offset);
    return flowi4_uli_offset;
}

__attribute__((always_inline)) u64 get_flowi6_saddr_offset() {
    u64 flowi6_saddr_offset;
    LOAD_CONSTANT("flowi6_saddr_offset", flowi6_saddr_offset);
    return flowi6_saddr_offset;
}

__attribute__((always_inline)) u64 get_flowi6_uli_offset() {
    u64 flowi6_uli_offset;
    LOAD_CONSTANT("flowi6_uli_offset", flowi6_uli_offset);
    return flowi6_uli_offset;
}

#endif
