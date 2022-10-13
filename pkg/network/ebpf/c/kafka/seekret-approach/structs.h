#pragma once

#include <linux/types.h>

#include "kconfig.h"
#include "bpf_telemetry.h"

#include "sockfd.h"

#include "bpf_endian.h"
#include "ip.h"
#include "ipv6.h"
#include "port.h"

#include <net/inet_sock.h>
#include <net/net_namespace.h>
#include <net/tcp_states.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/ptrace.h>
#include <uapi/linux/tcp.h>
#include <uapi/linux/udp.h>
#include <linux/err.h>

#include "enums.h"

struct protocol_message_t {
    enum traffic_protocol_t protocol;
    enum message_type_t type;
};

struct conn_id_t {
    uint32_t tgid;
    int32_t fd;
    uint64_t tsid; // Filled by bpf_ktime_get_ns function
};

union sockaddr_t {
    struct sockaddr sa;
    struct sockaddr_in in4;
    struct sockaddr_in6 in6;
};

struct payload_status_t {
    uint64_t payload_id;
    uint64_t payload_size;
    bool is_payload_malformed;
};

struct sock_metadata_t {
    u16 family;
    u16 sport;
    u16 dport;
    union {
        struct {
            // ipv4 addresses can be represented using a single u32. see struct `in_addr`.
            u32 saddr;
            u32 daddr;
        } ipv4;
        struct {
            struct in6_addr saddr;
            struct in6_addr daddr;
        } ipv6;
    };
} __attribute__((packed));

struct conn_info_t {
    struct conn_id_t conn_id;
    enum traffic_protocol_t protocol;
    enum endpoint_role_t role;
    int64_t wr_bytes;
    int64_t rd_bytes;
    struct payload_status_t current_payload_status;
    struct sock_metadata_t sock_meta;
    bool is_tls;
};

struct socket_data_event_t {
    struct attr_t {
        uint64_t timestamp_ns;
        struct conn_id_t conn_id;
        enum traffic_protocol_t protocol;
        enum endpoint_role_t role;
        enum traffic_direction_t direction;
        uint32_t msg_size;
        uint64_t pos;
        uint64_t payload_id;
        struct sock_metadata_t sock_metadata;
    } attr;
    char msg[30720];
};

struct socket_close_event_t {
    struct conn_id_t conn_id;
    uint64_t close_timestamp_ns;
    enum endpoint_role_t role;
    int64_t total_wr_bytes;
    int64_t total_rd_bytes;
};

struct malformed_socket_event_t {
    struct conn_id_t conn_id;
    uint64_t close_timestamp_ns;
    uint64_t payload_id;
};

// connect_args_t holds arguments used when calling the connect syscall
struct connect_args_t {
    const struct sockaddr* addr;
    int32_t fd;
    struct socket* sock_lookup_socket;
};

// accept_args_t holds arguments used when calling the accept syscall
struct accept_args_t {
    struct sockaddr* addr;
    struct socket* sock_alloc_socket;
};

struct data_args_t {
    int32_t fd;
    const char* buf;
    const struct iovec* iov;
    size_t iovlen;
    unsigned int msg_len;
};

struct tls_data_args_t {
    int32_t fd;
    const char* buf;
    size_t *tls_output_size;
};

struct close_args_t {
    int32_t fd;
};

struct bind_args_t {
    const struct sockaddr *addr;
};

// The key will be used to map SSL contexts to FDs.
struct tls_ctx_to_fd_key_t {
    uint64_t id;
    uint64_t tls_context_as_number;
};

struct tls_set_fd_args_t {
    int fd;
    void *tls_context;
};

struct http2_frame {
    uint32_t length;
    enum frame_type_t type;
    uint8_t flags;
    uint32_t stream_id;
};
