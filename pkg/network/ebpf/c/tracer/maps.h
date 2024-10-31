#ifndef __TRACER_MAPS_H
#define __TRACER_MAPS_H

#include "map-defs.h"
#include "bpf_helpers.h"

#include "conn_tuple.h"

/* This is a key/value store with the keys being a conn_tuple_t for send & recv calls
 * and the values being conn_stats_ts_t *.
 */
BPF_HASH_MAP(conn_stats, conn_tuple_t, conn_stats_ts_t, 0)

/* This is a key/value store with the keys being a conn_tuple_t
 * and the values being a tcp_stats_t *.
 */
BPF_HASH_MAP(tcp_stats, conn_tuple_t, tcp_stats_t, 0)

/*
 * Hash map to store conn_tuple_t to retransmits. We use a separate map
 * for retransmits from tcp_stats above since we don't normally
 * have the pid in the tcp_retransmit_skb kprobe
*/
BPF_HASH_MAP(tcp_retransmits, conn_tuple_t, __u32, 0)

/* Will hold the PIDs initiating TCP connections keyed by socket + tuple. PIDs have a timestamp attached so they can age out */
BPF_HASH_MAP(tcp_ongoing_connect_pid, skp_conn_tuple_t, pid_ts_t, 0)

/* Will hold a flag to indicate that closed connections have already been flushed */
BPF_HASH_MAP(conn_close_flushed, conn_tuple_t, __u64, 16384)

/* Will hold the tcp/udp close events
 * The keys are the cpu number and the values a perf file descriptor for a perf event
 */
BPF_PERF_EVENT_ARRAY_MAP(conn_close_event, __u32)

/* Will hold TCP failed connections */
BPF_PERF_EVENT_ARRAY_MAP(conn_fail_event, __u32)

/* We use this map as a container for batching closed tcp/udp connections
 * The key represents the CPU core. Ideally we should use a BPF_MAP_TYPE_PERCPU_HASH map
 * or BPF_MAP_TYPE_PERCPU_ARRAY, but they are not available in
 * some of the Kernels we support (4.4 ~ 4.6)
 */
BPF_HASH_MAP(conn_close_batch, __u32, batch_t, 1024)

/*
 * Map to hold struct sock parameter for tcp_sendmsg calls
 * to be used in kretprobe/tcp_sendmsg
 */
BPF_HASH_MAP(tcp_sendmsg_args, __u64, struct sock *, 1024)

/*
 * Map to hold struct sock parameter for tcp_sendpage calls
 * to be used in kretprobe/tcp_sendpage
 */
BPF_HASH_MAP(tcp_sendpage_args, __u64, struct sock *, 1024)

/*
 * Map to hold struct sock parameter for udp_sendpage calls
 * to be used in kretprobe/udp_sendpage
 */
BPF_HASH_MAP(udp_sendpage_args, __u64, struct sock *, 1024)

/*
 * Map to hold struct sock parameter for tcp_recvmsg/tcp_read_sock calls
 * to be used in kretprobe/tcp_recvmsg/tcp_read_sock
 */
BPF_HASH_MAP(tcp_recvmsg_args, __u64, struct sock *, 1024)

/* This map is used to match the kprobe & kretprobe of udp_recvmsg */
/* This is a key/value store with the keys being a pid
 * and the values being a udp_recv_sock_t
 */
BPF_HASH_MAP(udp_recv_sock, __u64, udp_recv_sock_t, 1024)

/* This map is used to match the kprobe & kretprobe of udpv6_recvmsg */
/* This is a key/value store with the keys being a pid
 * and the values being a udp_recv_sock_t
 */
BPF_HASH_MAP(udpv6_recv_sock, __u64, udp_recv_sock_t, 1024)

/* This maps tracks listening TCP ports. Entries are added to the map via tracing the inet_csk_accept syscall.  The
 * key in the map is the network namespace inode together with the port and the value is a flag that
 * indicates if the port is listening or not. When the socket is destroyed (via tcp_v4_destroy_sock), we set the
 * value to be "port closed" to indicate that the port is no longer being listened on.  We leave the data in place
 * for the userspace side to read and clean up
 */
BPF_HASH_MAP(port_bindings, port_binding_t, __u32, 0)

/* This behaves the same as port_bindings, except it tracks UDP ports.
 * Key: a port
 * Value: one of PORT_CLOSED, and PORT_OPEN
 */
BPF_HASH_MAP(udp_port_bindings, port_binding_t, __u32, 0)

/* Similar to pending_sockets this is used for capturing state between the call and return of the bind() system call.
 *
 * Keys: the PID returned by bpf_get_current_pid_tgid()
 * Values: the args of the bind call  being instrumented.
 */
BPF_HASH_MAP(pending_bind, __u64, bind_syscall_args_t, 8192)

/* This map is used for telemetry in kernelspace
 * only key 0 is used
 * value is a telemetry object
 */
BPF_ARRAY_MAP(telemetry, telemetry_t, 1)

/* Similar to pending_sockets this is used for capturing state between the call and return of the tcp_retransmit_skb() system call.
 *
 * Keys: the PID returned by bpf_get_current_pid_tgid()
 * Values: the args of the tcp_retransmit_skb call being instrumented.
 */
BPF_HASH_MAP(pending_tcp_retransmit_skb, __u64, tcp_retransmit_skb_args_t, 8192)

// Used to store ip(6)_make_skb args to be used in the
// corresponding kretprobes
BPF_HASH_MAP(ip_make_skb_args, __u64, ip_make_skb_args_t, 1024)

// Maps skb connection tuple to socket connection tuple.
// On ingress, skb connection tuple is pre NAT, and socket connection tuple is post NAT, and on egress, the opposite.
// We track the lifecycle of socket using tracepoint net/net_dev_queue.
// Some protocol can be classified in a single direction (for example HTTP/2 can be classified only by the first 24 bytes
// sent on the hand shake), and if we have NAT, then the conn tuple we extract from sk_buff will be different than the
// one we extract from the sock object, and then we are not able to correctly classify those protocols.
// To overcome those problems, we save two maps that translates from conn tuple of sk_buff to conn tuple of sock* and vice
// versa (the vice versa is used for cleanup purposes).
BPF_HASH_MAP(conn_tuple_to_socket_skb_conn_tuple, conn_tuple_t, conn_tuple_t, 0)

// Map to hold conn_tuple_t parameter for tcp_close calls
// to be used in kretprobe/tcp_close.
BPF_HASH_MAP(tcp_close_args, __u64, conn_tuple_t, 1024)

// This program array is needed to bypass a memory limit on socket filters.
// There is a limitation on number of instructions can be attached to a socket filter,
// as we dispatching more protocols, we reached that limit, thus we workaround it
// by using tail call.
BPF_PROG_ARRAY(tcp_close_progs, 1)

#endif
