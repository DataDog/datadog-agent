#ifndef __SK_UDP_H
#define __SK_UDP_H

#include "bpf_helpers.h"

#include "defs.h"
#include "maps.h"
#include "sock.h"
#include "tracer/tracer.h"
#include "sk.h"

static __always_inline void update_stats_tuple4(struct bpf_sock *ctx) {
    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, ctx, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return;
    }
    sk_stats->direction = CONN_DIRECTION_INCOMING;

    if (ctx->src_port) sk_stats->tup.sport = ctx->src_port;
    if (ctx->dst_port) sk_stats->tup.dport = bpf_ntohl(ctx->dst_port);
    sk_stats->tup.metadata |= CONN_V4;
    if (ctx->src_ip4) {
        sk_stats->tup.saddr_h = 0;
        sk_stats->tup.saddr_l = ctx->src_ip4;
    }
    if (ctx->dst_ip4) {
        sk_stats->tup.daddr_h = 0;
        sk_stats->tup.daddr_l = ctx->dst_ip4;
    }
}

static __always_inline void update_stats_tuple6(struct bpf_sock *ctx) {
    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, ctx, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return;
    }
    sk_stats->direction = CONN_DIRECTION_INCOMING;

    if (ctx->src_port) sk_stats->tup.sport = ctx->src_port;
    if (ctx->dst_port) sk_stats->tup.dport = bpf_ntohl(ctx->dst_port);
    sk_stats->tup.metadata |= CONN_V6;

    struct in6_addr saddr;
    saddr.in6_u.u6_addr32[0] = ctx->src_ip6[0];
    saddr.in6_u.u6_addr32[1] = ctx->src_ip6[1];
    saddr.in6_u.u6_addr32[2] = ctx->src_ip6[2];
    saddr.in6_u.u6_addr32[3] = ctx->src_ip6[3];
    read_in6_addr(&sk_stats->tup.saddr_h, &sk_stats->tup.saddr_l, &saddr);

    struct in6_addr daddr;
    daddr.in6_u.u6_addr32[0] = ctx->dst_ip6[0];
    daddr.in6_u.u6_addr32[1] = ctx->dst_ip6[1];
    daddr.in6_u.u6_addr32[2] = ctx->dst_ip6[2];
    daddr.in6_u.u6_addr32[3] = ctx->dst_ip6[3];
    read_in6_addr(&sk_stats->tup.daddr_h, &sk_stats->tup.daddr_l, &daddr);
}


SEC("cgroup/post_bind4")
int udp_post_bind4_cgroup(struct bpf_sock *ctx) {
    if (ctx->type != SOCK_DGRAM || ctx->protocol != IPPROTO_UDP) {
        return 1;
    }
    log_debug("post_bind4: sk=%p", ctx);
    update_stats_tuple4(ctx);
    return 1;
}

SEC("cgroup/post_bind6")
int udp_post_bind6_cgroup(struct bpf_sock *ctx) {
    if (ctx->type != SOCK_DGRAM || ctx->protocol != IPPROTO_UDP) {
        return 1;
    }
    log_debug("post_bind6: sk=%p", ctx);
    update_stats_tuple6(ctx);
    return 1;
}

SEC("fexit/udp_sendpage")
int BPF_PROG(udp_sendpage_exit, struct sock *sk, struct page *page, int offset, size_t size, int flags, int sent) {
    if (sent < 0) {
        log_debug("fexit/udp_sendpage: err=%d", sent);
        return 0;
    }
    log_debug("udp_sendpage: sk=%p sent=%d", sk, sent);

//    u64 pid_tgid = bpf_get_current_pid_tgid();
//    log_debug("fexit/udp_sendpage: pid_tgid: %llu, sent: %d, sock: %p", pid_tgid, sent, sk);

    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }

    sk_stats->sent_packets += 1;
    sk_stats->sent_bytes += sent;
    sk_stats->timestamp_ns = bpf_ktime_get_ns();
    if (!sk_stats->start_ns) {
        sk_stats->start_ns = sk_stats->timestamp_ns;
    }

    if (!(sk_stats->flags & CONN_ASSURED)) {
        if (sk_stats->recv_bytes == 0 && sent > 0) {
            sk_stats->flags |= CONN_L_INIT;
        }
        // If a three-way "handshake" was established, we mark the connection as assured
        if (sk_stats->flags & CONN_L_INIT && sk_stats->recv_bytes > 0 && sent > 0) {
            sk_stats->flags |= CONN_ASSURED;
        }
    }
    return 0;
}

SEC("fexit/udpv6_sendmsg")
int BPF_PROG(udpv6_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int sent) {
    log_debug("udpv6_sendmsg: sk=%p sent=%d", sk, sent);
    if (sent <= 0) {
        return 0;
    }

    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }

    if (!(sk_stats->tup.daddr_h || sk_stats->tup.daddr_l || sk_stats->tup.dport)) {
        if (msg && msg->msg_name) {
            // TODO this is probably not correct to store in sk_stats since it is per-msg, not per-sock
            if (msg->msg_namelen >= sizeof(struct sockaddr_in6)) {
                struct sockaddr_in6 usin;
                bpf_core_read(&usin, sizeof(struct sockaddr_in6), msg->msg_name);
                if (usin.sin6_family == AF_INET6) {
                    read_in6_addr(&sk_stats->tup.daddr_h, &sk_stats->tup.daddr_l, &usin.sin6_addr);
                    sk_stats->tup.dport = bpf_ntohs(usin.sin6_port);
                }
            }
        } else {
            // connected UDP sock
            if (sk && sk->__sk_common.skc_state == TCP_ESTABLISHED) {
                read_daddr_v6(sk, &sk_stats->tup.daddr_h, &sk_stats->tup.daddr_l);
                sk_stats->tup.dport = bpf_ntohs(inet_sk(sk)->inet_dport);
            }
        }
    }

    sk_stats->sent_packets += 1;
    sk_stats->sent_bytes += sent;
    sk_stats->timestamp_ns = bpf_ktime_get_ns();
    if (!sk_stats->start_ns) {
        sk_stats->start_ns = sk_stats->timestamp_ns;
    }

    if (!(sk_stats->flags & CONN_ASSURED)) {
        if (sk_stats->recv_bytes == 0 && sent > 0) {
            sk_stats->flags |= CONN_L_INIT;
        }
        // If a three-way "handshake" was established, we mark the connection as assured
        if (sk_stats->flags & CONN_L_INIT && sk_stats->recv_bytes > 0 && sent > 0) {
            sk_stats->flags |= CONN_ASSURED;
        }
    }
    return 0;
}

SEC("fentry/udp_send_skb")
int BPF_PROG(udp_send_skb_entry, struct sk_buff *skb, struct flowi4 *fl4) {
    struct sock *sk = skb->sk;
    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    if (!sk_stats->tup.saddr_l) {
        sk_stats->tup.saddr_l = read_saddr_v4(sk);
        if (!sk_stats->tup.saddr_l) {
            sk_stats->tup.saddr_l = fl4->saddr;
        }
    }
    if (!sk_stats->tup.sport) {
        sk_stats->tup.sport = read_sport(sk);
        if (!sk_stats->tup.sport) {
            sk_stats->tup.sport = bpf_ntohs(fl4->fl4_sport);
        }
    }
    return 0;
}

SEC("fentry/udp_v6_send_skb")
int BPF_PROG(udp_v6_send_skb_entry, struct sk_buff *skb, struct flowi6 *fl6) {
    struct sock *sk = skb->sk;
    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    if (!(sk_stats->tup.saddr_h || sk_stats->tup.saddr_l)) {
        read_saddr_v6(sk, &sk_stats->tup.saddr_h, &sk_stats->tup.saddr_l);
        if (!(sk_stats->tup.saddr_h || sk_stats->tup.saddr_l)) {
            read_in6_addr(&sk_stats->tup.saddr_h, &sk_stats->tup.saddr_l, &fl6->saddr);
        }
    }
    if (!sk_stats->tup.sport) {
        sk_stats->tup.sport = read_sport(sk);
        if (!sk_stats->tup.sport) {
            sk_stats->tup.sport = bpf_ntohs(fl6->fl6_sport);
        }
    }
    return 0;
}

SEC("fexit/udp_sendmsg")
int BPF_PROG(udp_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int sent) {
    log_debug("udp_sendmsg: sk=%p sent=%d", sk, sent);
    if (sent <= 0) {
        return 0;
    }

    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }

    if (!sk_stats->tup.saddr_l) {
        sk_stats->tup.saddr_l = read_saddr_v4(sk);
    }
    if (!sk_stats->tup.sport) {
        sk_stats->tup.sport = read_sport(sk);
    }

    if (!(sk_stats->tup.daddr_l || sk_stats->tup.dport)) {
        if (msg && msg->msg_name) {
            // TODO this is probably not correct to store in sk_stats since it is per-msg, not per-sock
            if (msg->msg_namelen >= sizeof(struct sockaddr_in)) {
                struct sockaddr_in usin;
                bpf_core_read(&usin, sizeof(struct sockaddr_in), msg->msg_name);
                if (usin.sin_family == AF_INET) {
                    sk_stats->tup.daddr_l = usin.sin_addr.s_addr;
                    sk_stats->tup.dport = bpf_ntohs(usin.sin_port);
                }
            }
        } else {
            // connected UDP sock
            if (sk && sk->__sk_common.skc_state == TCP_ESTABLISHED) {
                struct inet_sock *inet = inet_sk(sk);
                sk_stats->tup.daddr_l = inet->inet_daddr;
                sk_stats->tup.dport = bpf_ntohs(inet->inet_dport);
            }
        }
    }

    sk_stats->sent_packets += 1;
    sk_stats->sent_bytes += sent;
    sk_stats->timestamp_ns = bpf_ktime_get_ns();
    if (!sk_stats->start_ns) {
        sk_stats->start_ns = sk_stats->timestamp_ns;
    }

    if (!(sk_stats->flags & CONN_ASSURED)) {
        if (sk_stats->recv_bytes == 0 && sent > 0) {
            sk_stats->flags |= CONN_L_INIT;
        }
        // If a three-way "handshake" was established, we mark the connection as assured
        if (sk_stats->flags & CONN_L_INIT && sk_stats->recv_bytes > 0 && sent > 0) {
            sk_stats->flags |= CONN_ASSURED;
        }
    }
    return 0;
}

static __always_inline int handle_skb_consume_udp(struct sock *sk, struct sk_buff *skb, int len) {
    if (len < 0) {
        // peeking or an error happened
        return 0;
    }

    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    if (!sk_stats->tup.pid) {
        sk_stats->tup.pid = GET_USER_MODE_PID(bpf_get_current_pid_tgid());
    }
    if (!sk_stats->tup.netns) {
        sk_stats->tup.netns = get_netns_from_sock(sk);
    }
    sk_stats->tup.metadata |= CONN_TYPE_UDP;

    // TODO do we need to differentiate UDP connections by tuple instead of socket?
    unsigned char *head = skb->head;
    if (!head) {
        log_debug("ERR reading head");
        return 0;
    }
    u16 net_head = skb->network_header;
    if (!net_head) {
        log_debug("ERR reading network_header");
        return 0;
    }
    struct iphdr iph;
    bpf_memset(&iph, 0, sizeof(struct iphdr));
    int ret = bpf_probe_read_kernel(&iph, sizeof(iph), (struct iphdr *)(head + net_head));
    if (ret) {
        log_debug("ERR reading iphdr");
        return 0;
    }
    if (iph.version == 4) {
        if (!is_udpv4_enabled()) {
            return 0;
        }
        sk_stats->tup.metadata |= CONN_V4;
        bpf_probe_read_kernel(&sk_stats->tup.saddr_l, sizeof(__be32), &iph.saddr);
        bpf_probe_read_kernel(&sk_stats->tup.daddr_l, sizeof(__be32), &iph.daddr);
        if (sk_stats->tup.saddr_l == 0 || sk_stats->tup.daddr_l == 0) {
            log_debug("ERR(skb_consume_udp.v4): src or dst addr not set src=%llu, dst=%llu", sk_stats->tup.saddr_l, sk_stats->tup.daddr_l);
        }
    } else if (iph.version == 6) {
        if (!is_udpv6_enabled()) {
            return 0;
        }
        sk_stats->tup.metadata |= CONN_V6;
        struct ipv6hdr ip6h;
        bpf_memset(&ip6h, 0, sizeof(struct ipv6hdr));
        ret = bpf_probe_read_kernel(&ip6h, sizeof(ip6h), (struct ipv6hdr *)(head + net_head));
        if (ret) {
            log_debug("ERR reading ipv6 hdr");
            return 0;
        }
        read_in6_addr(&sk_stats->tup.saddr_h, &sk_stats->tup.saddr_l, &ip6h.saddr);
        read_in6_addr(&sk_stats->tup.daddr_h, &sk_stats->tup.daddr_l, &ip6h.daddr);
        if (!(sk_stats->tup.saddr_h || sk_stats->tup.saddr_l)) {
            log_debug("ERR(skb_consume_udp.v6): src addr not set: src_l:%llu,src_h:%llu",
                sk_stats->tup.saddr_l, sk_stats->tup.saddr_h);
        }

        if (!(sk_stats->tup.daddr_h || sk_stats->tup.daddr_l)) {
            log_debug("ERR(skb_consume_udp.v6): dst addr not set: dst_l:%llu,dst_h:%llu",
                sk_stats->tup.daddr_l, sk_stats->tup.daddr_h);
        }
    }

    u16 trans_head = skb->transport_header;
    if (!trans_head) {
        log_debug("ERR reading trans_head");
        return 0;
    }
    struct udphdr udph;
    bpf_memset(&udph, 0, sizeof(struct udphdr));
    ret = bpf_probe_read_kernel(&udph, sizeof(udph), (struct udphdr *)(head + trans_head));
    if (ret) {
        log_debug("ERR reading udphdr ret=%d", ret);
        return 0;
    }

    sk_stats->tup.sport = bpf_ntohs(udph.source);
    sk_stats->tup.dport = bpf_ntohs(udph.dest);
    flip_tuple(&sk_stats->tup);

    if (sk_stats->tup.sport == 0 || sk_stats->tup.dport == 0) {
        log_debug("ERR(skb_consume_udp.v4): src/dst port not set: src:%d, dst:%d", sk_stats->tup.sport, sk_stats->tup.dport);
    }

    int data_len = (int)(bpf_ntohs(udph.len) - sizeof(struct udphdr));
    if (data_len <= 0) {
        log_debug("ERR(skb_consume_udp): error reading data_len ret=%d", data_len);
        return 0;
    }

    sk_stats->recv_bytes += data_len;
    sk_stats->recv_packets += 1;
    sk_stats->timestamp_ns = bpf_ktime_get_ns();
    if (!sk_stats->start_ns) {
        sk_stats->start_ns = sk_stats->timestamp_ns;
    }

    log_debug("skb_consume_udp: sk=%p recv=%d", sk, data_len);
    if (!(sk_stats->flags & CONN_ASSURED)) {
        if (sk_stats->sent_bytes == 0 && data_len > 0) {
            sk_stats->flags |= CONN_R_INIT;
        }
        // If a three-way "handshake" was established, we mark the connection as assured
        if (sk_stats->flags & CONN_R_INIT && sk_stats->sent_bytes > 0 && data_len > 0) {
            sk_stats->flags |= CONN_ASSURED;
        }
    }
    return 0;
}

SEC("fentry/skb_consume_udp")
int BPF_PROG(skb_consume_udp, struct sock *sk, struct sk_buff *skb, int len) {
    return handle_skb_consume_udp(sk, skb, len);
}

SEC("fexit/udp_destroy_sock")
int BPF_PROG(udp_destroy_sock_exit, struct sock *sk) {
    log_debug("udp_destroy_sock: sk=%p", sk);
    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, 0);
    conn_t conn = {};
    if (!create_udp_conn(&conn, sk, sk_stats, NULL)) {
        return 0;
    }
    bpf_ringbuf_output(&conn_close_event, &conn, sizeof(conn_t), get_ringbuf_flags(sizeof(conn_t)));
    return 0;
}

SEC("fexit/udpv6_destroy_sock")
int BPF_PROG(udpv6_destroy_sock_exit, struct sock *sk) {
    log_debug("udpv6_destroy_sock: sk=%p", sk);
    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, 0);
    conn_t conn = {};
    if (!create_udp_conn(&conn, sk, sk_stats, NULL)) {
        return 0;
    }
    bpf_ringbuf_output(&conn_close_event, &conn, sizeof(conn_t), get_ringbuf_flags(sizeof(conn_t)));
    return 0;
}

#endif
