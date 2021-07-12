#include "tracer.h"
#include "bpf_helpers.h"
#include "syscalls.h"
#include "ip.h"
#include "ipv6.h"
#include "http.h"

// TODO: Replace those by injected constants based on system configuration
// once we have port range detection merged into the codebase.
#define EPHEMERAL_RANGE_BEG 32768
#define EPHEMERAL_RANGE_END 60999
#define HTTPS_PORT 443

static __always_inline int is_ephemeral_port(u16 port) {
    return port >= EPHEMERAL_RANGE_BEG && port <= EPHEMERAL_RANGE_END;
}

SEC("socket/http_filter")
int socket__http_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;

    if (!read_conn_tuple_skb(skb, &skb_info)) {
        return 0;
    }

    // don't bother to inspect packet contents when there is no chance we're dealing with plain HTTP
    if (!(skb_info.tup.metadata&CONN_TYPE_TCP) || skb_info.tup.sport == HTTPS_PORT || skb_info.tup.dport == HTTPS_PORT) {
        return 0;
    }


    // src_port represents the source port number *before* normalization
    // for more context please refer to http-types.h comment on `owned_by_src_port` field
    u16 src_port = skb_info.tup.sport;

    // we normalize the tuple to always be (client, server),
    // so if sport is not in ephemeral port range we flip it
    if (!is_ephemeral_port(skb_info.tup.sport)) {
        flip_tuple(&skb_info.tup);
    }

    http_handle_packet(skb, &skb_info, src_port);

    return 0;
}

// This kprobe is used to send batch completion notification to userspace
// because perf events can't be sent from socket filter programs
SEC("kretprobe/tcp_sendmsg")
int kretprobe__tcp_sendmsg(struct pt_regs* ctx) {
    http_notify_batch(ctx);
    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
