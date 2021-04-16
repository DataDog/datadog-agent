#include "tracer.h"
#include "bpf_helpers.h"
#include "syscalls.h"
#include "ip.h"
#include "ipv6.h"
#include "http.h"

SEC("socket/http_filter")
int socket__http_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;

    if (!read_conn_tuple_skb(skb, &skb_info)) {
        return 0;
    }

    if (skb_info.tup.sport != 80 && skb_info.tup.sport != 8080 && skb_info.tup.dport != 80 && skb_info.tup.dport != 8080) {
        return 0;
    }

    if (skb_info.tup.sport == 80 || skb_info.tup.sport == 8080) {
        // Normalize tuple
        flip_tuple(&skb_info.tup);
    }

    http_handle_packet(skb, &skb_info);

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
