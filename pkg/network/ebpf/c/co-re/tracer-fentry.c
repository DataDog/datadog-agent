#include "kconfig.h"
#include "bpf_telemetry.h"
#include "tracer.h"

#include "tracer-events.h"
#include "tracer-maps.h"
#include "tracer-stats.h"
#include "tracer-telemetry.h"
#include "sockfd.h"

#include "bpf_endian.h"
#include "ip.h"
#include "ipv6.h"
#include "port.h"

#include <net/inet_sock.h>
#include <net/net_namespace.h>
#include <net/tcp_states.h>
#include <net/ip.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/ptrace.h>
#include <uapi/linux/tcp.h>
#include <uapi/linux/udp.h>
#include <linux/err.h>
#include <linux/tcp.h>

#include "sock.h"

BPF_PERCPU_HASH_MAP(udp_send_skb_args, u64, u64, 1024)
BPF_PERCPU_HASH_MAP(udp6_send_skb_args, u64, u64, 1024)

static __always_inline void handle_tcp_stats(conn_tuple_t *t, struct sock *sk, u8 state) {
    u32 rtt = BPF_CORE_READ(tcp_sk(sk), srtt_us);
    u32 rtt_var = BPF_CORE_READ(tcp_sk(sk), mdev_us);

    tcp_stats_t stats = { .retransmits = 0, .rtt = rtt, .rtt_var = rtt_var };
    if (state > 0) {
        stats.state_transitions = (1 << state);
    }
    update_tcp_stats(t, stats);
}

static __always_inline void get_tcp_segment_counts(struct sock *skp, __u32 *packets_in, __u32 *packets_out) {
    *packets_in = BPF_CORE_READ(tcp_sk(skp), segs_in);
    *packets_out = BPF_CORE_READ(tcp_sk(skp), segs_out);
}

SEC("fexit/tcp_sendmsg")
int BPF_PROG(tcp_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t size, int sent) {
    if (sent < 0) {
        log_debug("fexit/tcp_sendmsg: tcp_sendmsg err=%d\n", sent);
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/tcp_sendmsg: pid_tgid: %d, sent: %d, sock: %llx\n", pid_tgid, sent, sk);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk, 0);

    __u32 packets_in = 0;
    __u32 packets_out = 0;
    get_tcp_segment_counts(sk, &packets_in, &packets_out);

    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, sk);
}

SEC("fentry/tcp_cleanup_rbuf")
int BPF_PROG(tcp_cleanup_rbuf, struct sock *sk, int copied) {
    if (copied <= 0) {
        return 0;
    }
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fentry/tcp_cleanup_rbuf: pid_tgid: %d, copied: %d\n", pid_tgid, copied);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk, 0);
    return handle_message(&t, 0, copied, CONN_DIRECTION_UNKNOWN, 0, 0, PACKET_COUNT_NONE, sk);
}

SEC("fentry/tcp_close")
int BPF_PROG(tcp_close, struct sock *sk, long timeout) {
    conn_tuple_t t = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Should actually delete something only if the connection never got established
    bpf_map_delete_elem(&tcp_ongoing_connect_pid, &sk);

    clear_sockfd_maps(sk);

    // Get network namespace id
    log_debug("fentry/tcp_close: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }
    log_debug("fentry/tcp_close: netns: %u, sport: %u, dport: %u\n", t.netns, t.sport, t.dport);

    cleanup_conn(&t);
    return 0;
}

SEC("fexit/tcp_close")
int BPF_PROG(tcp_close_exit, struct sock *sk, long timeout) {
    flush_conn_close_if_full(ctx);
    return 0;
}

static __always_inline int handle_ip6_skb(struct sock *sk, size_t size, struct flowi6 *fl6) {
    return 0;
}

SEC("fentry/udp_v6_send_skb")
int BPF_PROG(udp_v6_send_skb,
             struct sk_buff *skb, struct flowi6 *fl6,
             void *cork, int err) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 len = skb->len - skb_transport_offset(skb) - sizeof(struct udphdr);
    bpf_map_update_with_telemetry(udp6_send_skb_args, &pid_tgid, &len, BPF_ANY);
    return 0;
}

SEC("fexit/udp_v6_send_skb")
int BPF_PROG(udp_v6_send_skb_exit,
             struct sk_buff *skb, struct flowi6 *fl6,
             void *cork, int err) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 *len = bpf_map_lookup_elem(&udp6_send_skb_args, &pid_tgid);
    if (!len) {
        return 0;
    }

    if (err) {
        goto cleanup;
    }

    conn_tuple_t t = {};
    struct sock *sk = BPF_CORE_READ(skb, sk);
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP) &&
        !read_conn_tuple_partial_from_flowi6(&t, fl6, pid_tgid, CONN_TYPE_UDP)) {
        increment_telemetry_count(udp_send_missed);
        return 0;
    }

    log_debug("fexit/udp_v6_send_skb: pid_tgid: %d, size: %d\n", pid_tgid, *len);
    handle_message(&t, *len, 0, CONN_DIRECTION_UNKNOWN, 0, 0, PACKET_COUNT_NONE, sk);
    increment_telemetry_count(udp_send_processed);

 cleanup:
    bpf_map_delete_elem(&udp6_send_skb_args, &pid_tgid);
    return 0;
}

SEC("fentry/udp_send_skb")
int BPF_PROG(udp_send_skb,
             struct sk_buff *skb,
             struct flowi4 *fl4,
             void *cork,
             int err) {
    u64 len = skb->len - skb_transport_offset(skb) - sizeof(struct udphdr);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_update_with_telemetry(udp_send_skb_args, &pid_tgid, &len, BPF_ANY);

    return 0;
}

SEC("fexit/udp_send_skb")
int BPF_PROG(udp_send_skb_exit,
             struct sk_buff *skb,
             struct flowi4 *fl4,
             void *cork,
             int err) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 *len = bpf_map_lookup_elem(&udp_send_skb_args, &pid_tgid);
    if (!len) {
        return 0;
    }

    if (err) {
        goto cleanup;
    }

    conn_tuple_t t = {};
    struct sock *sk = BPF_CORE_READ(skb, sk);
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP) &&
        !read_conn_tuple_partial_from_flowi4(&t, fl4, pid_tgid, CONN_TYPE_UDP)) {
        increment_telemetry_count(udp_send_missed);
        return 0;
    }

    log_debug("fexit/udp_send_skb: pid_tgid: %d, size: %d\n", pid_tgid, *len);

    // segment count is not currently enabled on prebuilt.
    // to enable, change PACKET_COUNT_NONE => PACKET_COUNT_INCREMENT
    handle_message(&t, *len, 0, CONN_DIRECTION_UNKNOWN, 1, 0, PACKET_COUNT_NONE, sk);
    increment_telemetry_count(udp_send_processed);

 cleanup:
    bpf_map_delete_elem(&udp_send_skb_args, &pid_tgid);
    return 0;
}

static __always_inline int handle_ret_udp_recvmsg(struct sock *sk, struct msghdr *msg, int copied, int flags) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    if (copied < 0) { // Non-zero values are errors (or a peek) (e.g -EINVAL)
        log_debug("fexit/udp_recvmsg: ret=%d < 0, pid_tgid=%d\n", copied, pid_tgid);
        return 0;
    }

    if (flags & MSG_PEEK) {
        return 0;
    }

    log_debug("fexit/udp_recvmsg: ret=%d\n", copied);

    conn_tuple_t t = {};
    __builtin_memset(&t, 0, sizeof(conn_tuple_t));
    if (msg) {
        sockaddr_to_addr(msg->msg_name, &t.daddr_h, &t.daddr_l, &t.dport, &t.metadata);
    }

    if (!read_conn_tuple_partial(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
        log_debug("ERR(fexit/udp_recvmsg): error reading conn tuple, pid_tgid=%d\n", pid_tgid);
        return 0;
    }

    log_debug("fexit/udp_recvmsg: pid_tgid: %d, return: %d\n", pid_tgid, copied);
    // segment count is not currently enabled on prebuilt.
    // to enable, change PACKET_COUNT_NONE => PACKET_COUNT_INCREMENT
    handle_message(&t, 0, copied, CONN_DIRECTION_UNKNOWN, 0, 1, PACKET_COUNT_NONE, sk);

    return 0;
}

SEC("fexit/udp_recvmsg")
int BPF_PROG(udp_recvmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int noblock,
             int flags, int *addr_len,
             int copied) {
    return handle_ret_udp_recvmsg(sk, msg, copied, flags);
}

SEC("fexit/udpv6_recvmsg")
int BPF_PROG(udpv6_recvmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int noblock,
             int flags, int *addr_len,
             int copied) {
    return handle_ret_udp_recvmsg(sk, msg, copied, flags);
}

SEC("fentry/tcp_retransmit_skb")
int BPF_PROG(tcp_retransmit_skb, struct sock *sk, struct sk_buff *skb, int segs) {
    log_debug("fentry/tcp_retransmit: segs: %d\n", segs);
    return handle_retransmit(sk, segs);
}

SEC("fentry/tcp_set_state")
int BPF_PROG(tcp_set_state, struct sock *sk, int state) {
    // For now we're tracking only TCP_ESTABLISHED
    if (state != TCP_ESTABLISHED) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    tcp_stats_t stats = { .state_transitions = (1 << state) };
    update_tcp_stats(&t, stats);

    return 0;
}

SEC("fentry/tcp_connect")
int BPF_PROG(tcp_connect, struct sock *sk) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fentry/tcp_connect: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    bpf_map_update_with_telemetry(tcp_ongoing_connect_pid, &sk, &pid_tgid, BPF_ANY);

    return 0;
}

SEC("fentry/tcp_finish_connect")
int BPF_PROG(tcp_finish_connect, struct sock *sk, struct sk_buff *skb, int rc) {
    u64 *pid_tgid_p = bpf_map_lookup_elem(&tcp_ongoing_connect_pid, &sk);
    if (!pid_tgid_p) {
        return 0;
    }

    u64 pid_tgid = *pid_tgid_p;
    bpf_map_delete_elem(&tcp_ongoing_connect_pid, &sk);
    log_debug("fentry/tcp_finish_connect: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk, TCP_ESTABLISHED);
    handle_message(&t, 0, 0, CONN_DIRECTION_OUTGOING, 0, 0, PACKET_COUNT_NONE, sk);

    log_debug("fentry/tcp_connect: netns: %u, sport: %u, dport: %u\n", t.netns, t.sport, t.dport);

    return 0;
}

SEC("fexit/inet_csk_accept")
int BPF_PROG(inet_csk_accept_exit, struct sock *_sk, int flags, int *err, bool kern, struct sock *sk) {
    if (sk == NULL) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/inet_csk_accept: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }
    handle_tcp_stats(&t, sk, TCP_ESTABLISHED);
    handle_message(&t, 0, 0, CONN_DIRECTION_INCOMING, 0, 0, PACKET_COUNT_NONE, sk);

    port_binding_t pb = {};
    pb.netns = t.netns;
    pb.port = t.sport;
    add_port_bind(&pb, port_bindings);
    log_debug("fexit/inet_csk_accept: netns: %u, sport: %u, dport: %u\n", t.netns, t.sport, t.dport);
    return 0;
}

SEC("fentry/inet_csk_listen_stop")
int BPF_PROG(inet_csk_listen_stop_enter, struct sock *sk) {
    __u16 lport = read_sport(sk);
    if (lport == 0) {
        log_debug("ERR(inet_csk_listen_stop): lport is 0 \n");
        return 0;
    }

    port_binding_t pb = {};
    pb.netns = get_netns_from_sock(sk);
    pb.port = lport;
    remove_port_bind(&pb, &port_bindings);
    log_debug("fentry/inet_csk_listen_stop: net ns: %u, lport: %u\n", pb.netns, pb.port);
    return 0;
}

SEC("fentry/udp_destroy_sock")
int BPF_PROG(udp_destroy_sock, struct sock *sk) {
    conn_tuple_t tup = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int valid_tuple = read_conn_tuple(&tup, sk, pid_tgid, CONN_TYPE_UDP);

    __u16 lport = 0;
    if (valid_tuple) {
        cleanup_conn(&tup);
        lport = tup.sport;
    } else {
        // get the port for the current sock
        lport = read_sport(sk);
    }

    if (lport == 0) {
        log_debug("ERR(udp_destroy_sock): lport is 0\n");
        return 0;
    }

    // although we have net ns info, we don't use it in the key
    // since we don't have it everywhere for udp port bindings
    // (see sys_enter_bind/sys_exit_bind below)
    port_binding_t pb = {};
    pb.netns = 0;
    pb.port = lport;
    remove_port_bind(&pb, &udp_port_bindings);

    log_debug("fentry/udp_destroy_sock: port %d marked as closed\n", lport);

    return 0;
}

SEC("fexit/udp_destroy_sock")
int BPF_PROG(udp_destroy_sock_exit, struct sock *sk) {
    flush_conn_close_if_full(ctx);
    return 0;
}

//region sys_exit_bind

static __always_inline int sys_exit_bind(struct socket *sock, struct sockaddr *addr, int rc) {
    if (rc != 0) {
        return 0;
    }

    __u16 type = 0;
    bpf_probe_read_with_telemetry(&type, sizeof(type), &sock->type);
    if ((type & SOCK_DGRAM) == 0) {
        return 0;
    }

    if (addr == NULL) {
        log_debug("sys_enter_bind: could not read sockaddr, sock=%llx\n", sock);
        return 0;
    }

    u16 sin_port = 0;
    sa_family_t family = 0;
    bpf_probe_read_kernel_with_telemetry(&family, sizeof(sa_family_t), &addr->sa_family);
    if (family == AF_INET) {
        bpf_probe_read_kernel_with_telemetry(&sin_port, sizeof(u16), &(((struct sockaddr_in *)addr)->sin_port));
    } else if (family == AF_INET6) {
        bpf_probe_read_kernel_with_telemetry(&sin_port, sizeof(u16), &(((struct sockaddr_in6 *)addr)->sin6_port));
    }

    sin_port = ntohs(sin_port);
    if (sin_port == 0) {
        log_debug("ERR(sys_enter_bind): sin_port is 0\n");
        return 0;
    }

    port_binding_t pb = {};
    pb.netns = 0; // don't have net ns info in this context
    pb.port = sin_port;
    add_port_bind(&pb, udp_port_bindings);
    log_debug("sys_exit_bind: bound UDP port %u\n", sin_port);

    return 0;
}

SEC("fexit/inet_bind")
int BPF_PROG(inet_bind_exit, struct socket *sock, struct sockaddr *uaddr, int addr_len, int rc) {
    log_debug("fexit/inet_bind: rc=%d\n", rc);
    return sys_exit_bind(sock, uaddr, rc);
}

SEC("fexit/inet6_bind")
int BPF_PROG(inet6_bind_exit, struct socket *sock, struct sockaddr *uaddr, int addr_len, int rc) {
    log_debug("fexit/inet6_bind: rc=%d\n", rc);
    return sys_exit_bind(sock, uaddr, rc);
}

// this kretprobe is essentially creating:
// * an index of pid_fd_t to a struct sock*;
// * an index of struct sock* to pid_fd_t;
SEC("fexit/sockfd_lookup_light")
int BPF_PROG(sockfd_lookup_light_exit, int fd, int *err, int *fput_needed, struct socket *socket) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    // Check if have already a map entry for this pid_fd_t
    // TODO: This lookup eliminates *4* map operations for existing entries
    // but can reduce the accuracy of programs relying on socket FDs for
    // processes with a lot of FD churn
    pid_fd_t key = {
        .pid = pid_tgid >> 32,
        .fd = fd,
    };

    struct sock **skpp = bpf_map_lookup_elem(&sock_by_pid_fd, &key);
    if (skpp != NULL) {
        return 0;
    }

    // For now let's only store information for TCP sockets
    enum sock_type sock_type = 0;
    bpf_probe_read_kernel_with_telemetry(&sock_type, sizeof(short), &socket->type);

    const struct proto_ops *proto_ops = BPF_CORE_READ(socket, ops);
    if (!proto_ops) {
        return 0;
    }

    int family = 0;
    bpf_probe_read_kernel_with_telemetry(&family, sizeof(family), &proto_ops->family);
    if (sock_type != SOCK_STREAM || !(family == AF_INET || family == AF_INET6)) {
        return 0;
    }

    // Retrieve struct sock* pointer from struct socket*
    struct sock *sock = BPF_CORE_READ(socket, sk);

    pid_fd_t pid_fd = {
        .pid = pid_tgid >> 32,
        .fd = fd,
    };

    // These entries are cleaned up by tcp_close
    bpf_map_update_with_telemetry(pid_fd_by_sock, &sock, &pid_fd, BPF_ANY);
    bpf_map_update_with_telemetry(sock_by_pid_fd, &pid_fd, &sock, BPF_ANY);

    return 0;
}

SEC("fexit/do_sendfile")
int BPF_PROG(do_sendfile_exit, int out_fd, int in_fd, loff_t *ppos,
             size_t count, loff_t max, ssize_t sent) {
    if (sent <= 0) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    pid_fd_t key = {
        .pid = pid_tgid >> 32,
        .fd = out_fd,
    };
    struct sock **sock = bpf_map_lookup_elem(&sock_by_pid_fd, &key);
    if (sock == NULL) {
        return 0;
    }

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, *sock, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, 0, 0, PACKET_COUNT_NONE, *sock);

    return 0;
}

//endregion

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
