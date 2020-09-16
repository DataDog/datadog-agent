#include "tracer-ebpf.h"
#include "bpf_helpers.h"
#include "syscalls.h"
#include <linux/kconfig.h>
#include <net/inet_sock.h>
#include <net/net_namespace.h>
#include <net/tcp_states.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/ptrace.h>
#include <uapi/linux/tcp.h>
#include <uapi/linux/udp.h>

/* The LOAD_CONSTANT macro is used to define a named constant that will be replaced
 * at runtime by the Go code. This replaces usage of a bpf_map for storing values, which
 * eliminates a bpf_map_lookup_elem per kprobe hit. The constants are best accessed with a
 * dedicated inlined function. See example functions offset_* below.
 */
#define LOAD_CONSTANT(param, var) asm("%0 = " param " ll" \
                                      : "=r"(var))

enum telemetry_counter{tcp_sent_miscounts, missed_tcp_close, udp_send_processed, udp_send_missed};

/* This is a key/value store with the keys being a conn_tuple_t for send & recv calls
 * and the values being conn_stats_ts_t *.
 */
struct bpf_map_def SEC("maps/conn_stats") conn_stats = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(conn_tuple_t),
    .value_size = sizeof(conn_stats_ts_t),
    .max_entries = 0, // This will get overridden at runtime using max_tracked_connections
    .pinning = 0,
    .namespace = "",
};

/* This is a key/value store with the keys being a conn_tuple_t (but without the PID being used)
 * and the values being a tcp_stats_t *.
 */
struct bpf_map_def SEC("maps/tcp_stats") tcp_stats = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(conn_tuple_t),
    .value_size = sizeof(tcp_stats_t),
    .max_entries = 0, // This will get overridden at runtime using max_tracked_connections
    .pinning = 0,
    .namespace = "",
};

/* Will hold the tcp close events
 * The keys are the cpu number and the values a perf file descriptor for a perf event
 */
struct bpf_map_def SEC("maps/tcp_close_event") tcp_close_event = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 0, // This will get overridden at runtime
    .pinning = 0,
    .namespace = "",
};

/* We use this map as a container for batching closed tcp connections
 * The key represents the CPU core. Ideally we should use a BPF_MAP_TYPE_PERCPU_HASH map
 * or BPF_MAP_TYPE_PERCPU_ARRAY, but they are not available in
 * some of the Kernels we support (4.4 ~ 4.6)
 */
struct bpf_map_def SEC("maps/tcp_close_batch") tcp_close_batch = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u32),
    .value_size = sizeof(batch_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

/* This map is used to match the kprobe & kretprobe of udp_recvmsg */
/* This is a key/value store with the keys being a pid
 * and the values being a struct sock *.
 */
struct bpf_map_def SEC("maps/udp_recv_sock") udp_recv_sock = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(void*),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

/* This maps tracks listening TCP ports. Entries are added to the map via tracing the inet_csk_accept syscall.  The
 * key in the map is the port and the value is a flag that indicates if the port is listening or not.
 * When the socket is destroyed (via tcp_v4_destroy_sock), we set the value to be "port closed" to indicate that the
 * port is no longer being listened on.  We leave the data in place for the userspace side to read and clean up
 */
struct bpf_map_def SEC("maps/port_bindings") port_bindings = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u16),
    .value_size = sizeof(__u8),
    .max_entries = 0, // This will get overridden at runtime using max_tracked_connections
    .pinning = 0,
    .namespace = "",
};

/* This behaves the same as port_bindings, except it tracks UDP ports.
 * Key: a port
 * Value: one of PORT_CLOSED, and PORT_OPEN
 */
struct bpf_map_def SEC("maps/udp_port_bindings") udp_port_bindings = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u16),
    .value_size = sizeof(__u8),
    .max_entries = 0, // This will get overridden at runtime using max_tracked_connections
    .pinning = 0,
    .namespace = "",
};

/* This is used purely for capturing state between the call and return of the socket() system call.
 * When a sys_socket kprobe fires, we only have access to the params, which can tell us if the socket is using
 * SOCK_DGRAM or not. The kretprobe will only tell us the returned file descriptor.
 *
 * Keys: the PID returned by bpf_get_current_pid_tgid().
 * Value: 1 if the PID is mid-call to socket() and the call is creating a UDP socket, else there will be no entry.
 */
struct bpf_map_def SEC("maps/pending_sockets") pending_sockets = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(__u8),
    .max_entries = 8192,
    .pinning = 0,
    .namespace = "",
};

/* Similar to pending_sockets this is used for capturing state between the call and return of the bind() system call.
 *
 * Keys: the PId returned by bpf_get_current_pid_tgid()
 * Values: the args of the bind call  being instrumented.
 */
struct bpf_map_def SEC("maps/pending_bind") pending_bind = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(bind_syscall_args_t),
    .max_entries = 8192,
    .pinning = 0,
    .namespace = "",
};

/* This is written to in the kretprobe for sys_socket to keep track of
 * sockets that were created, but have not yet been bound to a port with
 * sys_bind.
 *
 * Key: a __u64 where the upper 32 bits are the PID of the process which created the socket, and the lower
 * 32 bits are the file descriptor as returned by socket().
 * Value: the values are not relevant. It's only relevant that there is or isn't an entry.
 *
 */
struct bpf_map_def SEC("maps/unbound_sockets") unbound_sockets = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(__u8),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

/* This map is used for telemetry in kernelspace
 * only key 0 is used
 * value is a telemetry object
 */
struct bpf_map_def SEC("maps/telemetry") telemetry = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u16),
    .value_size = sizeof(telemetry_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

/* http://stackoverflow.com/questions/1001307/detecting-endianness-programmatically-in-a-c-program */
static __always_inline bool is_big_endian(void) {
    union {
        uint32_t i;
        char c[4];
    } bint = { 0x01020304 };

    return bint.c[0] == 1;
}

/* check if IPs are IPv4 mapped to IPv6 ::ffff:xxxx:xxxx
 * https://tools.ietf.org/html/rfc4291#section-2.5.5
 * the addresses are stored in network byte order so IPv4 adddress is stored
 * in the most significant 32 bits of part saddr_l and daddr_l.
 * Meanwhile the end of the mask is stored in the least significant 32 bits.
 */
static __always_inline bool is_ipv4_mapped_ipv6(u64 saddr_h, u64 saddr_l, u64 daddr_h, u64 daddr_l) {
    if (is_big_endian()) {
        return ((saddr_h == 0 && ((u32)(saddr_l >> 32) == 0x0000FFFF)) || (daddr_h == 0 && ((u32)(daddr_l >> 32) == 0x0000FFFF)));
    } else {
        return ((saddr_h == 0 && ((u32)saddr_l == 0xFFFF0000)) || (daddr_h == 0 && ((u32)daddr_l == 0xFFFF0000)));
    }
}

static __always_inline bool dns_stats_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("dns_stats_enabled", val);
    return val == 1;
}

static __always_inline __u64 offset_family() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_family", val);
    return val;
}

static __always_inline __u64 offset_saddr() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_saddr", val);
    return val;
}

static __always_inline __u64 offset_daddr() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_daddr", val);
    return val;
}

static __always_inline __u64 offset_daddr_ipv6() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_daddr_ipv6", val);
    return val;
}

static __always_inline __u64 offset_sport() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_sport", val);
    return val;
}

static __always_inline __u64 offset_dport() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_dport", val);
    return val;
}

static __always_inline __u64 offset_netns() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_netns", val);
    return val;
}

static __always_inline __u64 offset_ino() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_ino", val);
    return val;
}

static __always_inline __u64 offset_rtt() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_rtt", val);
    return val;
}

static __always_inline __u64 offset_rtt_var() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_rtt_var", val);
    return val;
}

static __always_inline bool is_ipv6_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("ipv6_enabled", val);
    return val == ENABLED;
}

static __always_inline bool are_fl4_offsets_known() {
    __u64 val = 0;
    LOAD_CONSTANT("fl4_offsets", val);
    return val == ENABLED;
}

static __always_inline __u64 offset_saddr_fl4() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_saddr_fl4", val);
    return val;
}

static __always_inline __u64 offset_daddr_fl4() {
     __u64 val = 0;
     LOAD_CONSTANT("offset_daddr_fl4", val);
     return val;
}

static __always_inline __u64 offset_sport_fl4() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_sport_fl4", val);
    return val;
}

static __always_inline __u64 offset_dport_fl4() {
     __u64 val = 0;
     LOAD_CONSTANT("offset_dport_fl4", val);
     return val;
}

static __always_inline bool check_family(struct sock* sk, u16 expected_family) {
    u16 family = 0;
    bpf_probe_read(&family, sizeof(u16), ((char*)sk) + offset_family());
    return family == expected_family;
}

static __always_inline int read_conn_tuple(conn_tuple_t* t, struct sock* skp, u64 pid_tgid, metadata_mask_t type) {
    t->saddr_h = 0;
    t->saddr_l = 0;
    t->daddr_h = 0;
    t->daddr_l = 0;
    t->sport = 0;
    t->dport = 0;
    t->pid = pid_tgid >> 32;
    t->metadata = type;

    // Retrieve network namespace id first since addresses and ports may not be available for unconnected UDP
    // sends
    possible_net_t* skc_net = NULL;
    bpf_probe_read(&skc_net, sizeof(void*), ((char*)skp) + offset_netns());
    bpf_probe_read(&t->netns, sizeof(t->netns), ((char*)skc_net) + offset_ino());

    // Retrieve addresses
    if (check_family(skp, AF_INET)) {
        t->metadata |= CONN_V4;
        bpf_probe_read(&t->saddr_l, sizeof(u32), ((char*)skp) + offset_saddr());
        bpf_probe_read(&t->daddr_l, sizeof(u32), ((char*)skp) + offset_daddr());

        if (!t->saddr_l || !t->daddr_l) {
            log_debug("ERR(read_conn_tuple.v4): src/dst addr not set src:%d,dst:%d\n", t->saddr_l, t->daddr_l);
            return 0;
        }
    } else if (is_ipv6_enabled() && check_family(skp, AF_INET6)) {
        bpf_probe_read(&t->saddr_h, sizeof(t->saddr_h), ((char*)skp) + offset_daddr_ipv6() + 2 * sizeof(u64));
        bpf_probe_read(&t->saddr_l, sizeof(t->saddr_l), ((char*)skp) + offset_daddr_ipv6() + 3 * sizeof(u64));
        bpf_probe_read(&t->daddr_h, sizeof(t->daddr_h), ((char*)skp) + offset_daddr_ipv6());
        bpf_probe_read(&t->daddr_l, sizeof(t->daddr_l), ((char*)skp) + offset_daddr_ipv6() + sizeof(u64));

        // We can only pass 4 args to bpf_trace_printk
        // so split those 2 statements to be able to log everything
        if (!(t->saddr_h || t->saddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): src addr not set: src_l:%d,src_h:%d\n",
                t->saddr_l, t->saddr_h);
            return 0;
        }

        if (!(t->daddr_h || t->daddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): dst addr not set: dst_l:%d,dst_h:%d\n",
                t->daddr_l, t->daddr_h);
            return 0;
        }

        // Check if we can map IPv6 to IPv4
        if (is_ipv4_mapped_ipv6(t->saddr_h, t->saddr_l, t->daddr_h, t->daddr_l)) {
            t->metadata |= CONN_V4;
            t->saddr_h = 0;
            t->daddr_h = 0;
            t->saddr_l = (u32)(t->saddr_l >> 32);
            t->daddr_l = (u32)(t->daddr_l >> 32);
        } else {
            t->metadata |= CONN_V6;
        }
    }

    // Retrieve ports
    bpf_probe_read(&t->sport, sizeof(t->sport), ((char*)skp) + offset_sport());
    bpf_probe_read(&t->dport, sizeof(t->dport), ((char*)skp) + offset_dport());

    if (t->sport == 0 || t->dport == 0) {
        log_debug("ERR(read_conn_tuple.v4): src/dst port not set: src:%d, dst:%d\n", t->sport, t->dport);
        return 0;
    }

    // Making ports human-readable
    t->sport = ntohs(t->sport);
    t->dport = ntohs(t->dport);

    return 1;
}

static __always_inline void update_conn_stats(conn_tuple_t* t, size_t sent_bytes, size_t recv_bytes, u64 ts) {
    conn_stats_ts_t* val;

    // initialize-if-no-exist the connection stat, and load it
    conn_stats_ts_t empty = {};
    bpf_map_update_elem(&conn_stats, t, &empty, BPF_NOEXIST);
    val = bpf_map_lookup_elem(&conn_stats, t);

    // If already in our map, increment size in-place
    if (val != NULL) {
        if (sent_bytes) {
            __sync_fetch_and_add(&val->sent_bytes, sent_bytes);
        }
        if (recv_bytes) {
            __sync_fetch_and_add(&val->recv_bytes, recv_bytes);
        }
        val->timestamp = ts;
    }
}

static __always_inline void update_tcp_stats(conn_tuple_t* t, tcp_stats_t stats) {
    // query stats without the PID from the tuple
    u32 pid = t->pid;
    t->pid = 0;

    // initialize-if-no-exist the connetion state, and load it
    tcp_stats_t empty = {};
    bpf_map_update_elem(&tcp_stats, t, &empty, BPF_NOEXIST);

    tcp_stats_t* val = bpf_map_lookup_elem(&tcp_stats, t);
    t->pid = pid;
    if (val == NULL) {
        return;
    }

    if (stats.retransmits > 0) {
        __sync_fetch_and_add(&val->retransmits, stats.retransmits);
    }

    if (stats.rtt > 0) {
        // For more information on the bit shift operations see:
        // https://elixir.bootlin.com/linux/v4.6/source/net/ipv4/tcp.c#L2686
        val->rtt = stats.rtt >> 3;
        val->rtt_var = stats.rtt_var >> 2;
    }

    if (stats.state_transitions > 0) {
        val->state_transitions |= stats.state_transitions;
    }
}

static __always_inline void increment_telemetry_count(enum telemetry_counter counter_name) {
    __u64 key = 0;
    telemetry_t empty = {};
    telemetry_t* val;
    bpf_map_update_elem(&telemetry, &key, &empty, BPF_NOEXIST);
    val = bpf_map_lookup_elem(&telemetry, &key);

    if (val == NULL) {
        return;
    }
    switch (counter_name) {
        case tcp_sent_miscounts:
            __sync_fetch_and_add(&val->tcp_sent_miscounts, 1);
            break;
        case missed_tcp_close:
            __sync_fetch_and_add(&val->missed_tcp_close, 1);
            break;
        case udp_send_processed:
            __sync_fetch_and_add(&val->udp_sends_processed, 1);
            break;
        case udp_send_missed:
            __sync_fetch_and_add(&val->udp_sends_missed, 1);
            break;
    }
    return;
}

static __always_inline void cleanup_tcp_conn(struct pt_regs* __attribute__((unused)) ctx, conn_tuple_t* tup) {
    u32 cpu = bpf_get_smp_processor_id();

    // Will hold the full connection data to send through the perf buffer
    tcp_conn_t conn = {};
    bpf_probe_read(&(conn.tup), sizeof(conn_tuple_t), tup);
    tcp_stats_t* tst;
    conn_stats_ts_t* cst;

    // TCP stats don't have the PID
    conn.tup.pid = 0;
    tst = bpf_map_lookup_elem(&tcp_stats, &(conn.tup));
    bpf_map_delete_elem(&tcp_stats, &(conn.tup));
    conn.tup.pid = tup->pid;

    cst = bpf_map_lookup_elem(&conn_stats, &(conn.tup));
    // Delete this connection from our stats map
    bpf_map_delete_elem(&conn_stats, &(conn.tup));

    if (tst != NULL) {
        conn.tcp_stats = *tst;
    }
    conn.tcp_stats.state_transitions |= (1 << TCP_CLOSE);

    if (cst != NULL) {
        cst->timestamp = bpf_ktime_get_ns();
        conn.conn_stats = *cst;
    }

    // Batch TCP closed connections before generating a perf event
    batch_t* batch_ptr = bpf_map_lookup_elem(&tcp_close_batch, &cpu);
    if (batch_ptr == NULL) {
        return;
    }

    // TODO: Can we turn this into a macro based on TCP_CLOSED_BATCH_SIZE?
    switch (batch_ptr->pos) {
    case 0:
        batch_ptr->c0 = conn;
        batch_ptr->pos++;
        return;
    case 1:
        batch_ptr->c1 = conn;
        batch_ptr->pos++;
        return;
    case 2:
        batch_ptr->c2 = conn;
        batch_ptr->pos++;
        return;
    case 3:
        batch_ptr->c3 = conn;
        batch_ptr->pos++;
        return;
    case 4:
        // In this case the batch is ready to be flushed, which we defer to kretprobe/tcp_close
        // in order to cope with the eBPF stack limitation of 512 bytes.
        batch_ptr->c4 = conn;
        batch_ptr->pos++;
        return;
    }

    // If we hit this section it means we had one or more interleaved tcp_close calls.
    // This could result in a missed tcp_close event, so we track it using our telemetry map.
    increment_telemetry_count(missed_tcp_close);
}

static __always_inline int handle_message(conn_tuple_t* t, size_t sent_bytes, size_t recv_bytes) {
    u64 ts = bpf_ktime_get_ns();

    update_conn_stats(t, sent_bytes, recv_bytes, ts);

    return 0;
}

static __always_inline int handle_retransmit(struct sock* sk) {
    conn_tuple_t t = {};
    u64 zero = 0;

    if (!read_conn_tuple(&t, sk, zero, CONN_TYPE_TCP)) {
        return 0;
    }

    tcp_stats_t stats = { .retransmits = 1, .rtt = 0, .rtt_var = 0 };
    update_tcp_stats(&t, stats);

    return 0;
}

static __always_inline void handle_tcp_stats(conn_tuple_t* t, struct sock* sk) {
    u32 rtt = 0, rtt_var = 0;
    bpf_probe_read(&rtt, sizeof(rtt), ((char*)sk) + offset_rtt());
    bpf_probe_read(&rtt_var, sizeof(rtt_var), ((char*)sk) + offset_rtt_var());

    tcp_stats_t stats = { .retransmits = 0, .rtt = rtt, .rtt_var = rtt_var };
    update_tcp_stats(t, stats);
}

SEC("kprobe/tcp_sendmsg")
int kprobe__tcp_sendmsg(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    size_t size = (size_t)PT_REGS_PARM3(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendmsg: pid_tgid: %d, size: %d\n", pid_tgid, size);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk);
    return handle_message(&t, size, 0);
}

SEC("kprobe/tcp_sendmsg/pre_4_1_0")
int kprobe__tcp_sendmsg__pre_4_1_0(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM2(ctx);
    size_t size = (size_t)PT_REGS_PARM4(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendmsg/pre_4_1_0: pid_tgid: %d, size: %d\n", pid_tgid, size);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk);
    return handle_message(&t, size, 0);
}

SEC("kretprobe/tcp_sendmsg")
int kretprobe__tcp_sendmsg(struct pt_regs* ctx) {
    int ret = PT_REGS_RC(ctx);

    log_debug("kretprobe/tcp_sendmsg: return: %d\n", ret);
    // If ret < 0 it means an error occurred but we still counted the bytes as being sent
    // let's increment our miscount count
    if (ret < 0) {
        increment_telemetry_count(tcp_sent_miscounts);
    }

    return 0;
}

SEC("kprobe/tcp_cleanup_rbuf")
int kprobe__tcp_cleanup_rbuf(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    int copied = (int)PT_REGS_PARM2(ctx);
    if (copied < 0) {
        return 0;
    }
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_cleanup_rbuf: pid_tgid: %d, copied: %d\n", pid_tgid, copied);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    return handle_message(&t, 0, copied);
}

SEC("kprobe/tcp_close")
int kprobe__tcp_close(struct pt_regs* ctx) {
    struct sock* sk;
    conn_tuple_t t = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    sk = (struct sock*)PT_REGS_PARM1(ctx);

    u32 net_ns_inum;

    // Get network namespace id
    possible_net_t* skc_net;

    skc_net = NULL;
    net_ns_inum = 0;
    bpf_probe_read(&skc_net, sizeof(possible_net_t*), ((char*)sk) + offset_netns());
    bpf_probe_read(&net_ns_inum, sizeof(net_ns_inum), ((char*)skc_net) + offset_ino());

    log_debug("kprobe/tcp_close: pid_tgid: %d, ns: %d\n", pid_tgid, net_ns_inum);

    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    cleanup_tcp_conn(ctx, &t);
    return 0;
}

SEC("kretprobe/tcp_close")
int kretprobe__tcp_close(struct pt_regs* ctx) {
    u32 cpu = bpf_get_smp_processor_id();
    batch_t* batch_ptr = bpf_map_lookup_elem(&tcp_close_batch, &cpu);
    if (batch_ptr == NULL) {
        return 0;
    }

    if (batch_ptr->pos >= TCP_CLOSED_BATCH_SIZE) {
        // Here we copy the batch data to a variable allocated in the eBPF stack
        // This is necessary for older Kernel versions only (we validated this behavior on 4.4.0),
        // since you can't directly write a map entry to the perf buffer.
        batch_t batch_copy = {};
        __builtin_memcpy(&batch_copy, batch_ptr, sizeof(batch_copy));
        bpf_perf_event_output(ctx, &tcp_close_event, cpu, &batch_copy, sizeof(batch_copy));
        batch_ptr->pos = 0;
    }

    return 0;
}

SEC("kprobe/ip6_make_skb")
int kprobe__ip6_make_skb(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    size_t size = (size_t)PT_REGS_PARM4(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();

    size = size - sizeof(struct udphdr);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {

        increment_telemetry_count(udp_send_missed);
        return 0;
    }

    log_debug("kprobe/ip6_make_skb: pid_tgid: %d, size: %d\n", pid_tgid, size);
    handle_message(&t, size, 0);
    increment_telemetry_count(udp_send_processed);

    return 0;
}

// Note: This is used only in tne UDP send path.
SEC("kprobe/ip_make_skb")
int kprobe__ip_make_skb(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    size_t size = (size_t)PT_REGS_PARM5(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();

    size = size - sizeof(struct udphdr);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
        if (!are_fl4_offsets_known()) {
            log_debug("ERR: src/dst addr not set src:%d,dst:%d. fl4 offsets are not known\n", t.saddr_l, t.daddr_l);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        struct flowi4* fl4 = (struct flowi4*)PT_REGS_PARM2(ctx);
        bpf_probe_read(&t.saddr_l, sizeof(u32), ((char*)fl4) + offset_saddr_fl4());
        bpf_probe_read(&t.daddr_l, sizeof(u32), ((char*)fl4) + offset_daddr_fl4());

        if (!t.saddr_l || !t.daddr_l) {
            log_debug("ERR(fl4): src/dst addr not set src:%d,dst:%d\n", t.saddr_l, t.daddr_l);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        bpf_probe_read(&t.sport, sizeof(t.sport), ((char*)fl4) + offset_sport_fl4());
        bpf_probe_read(&t.dport, sizeof(t.dport), ((char*)fl4) + offset_dport_fl4());

        if (t.sport == 0 || t.dport == 0) {
            log_debug("ERR(fl4): src/dst port not set: src:%d, dst:%d\n", t.sport, t.dport);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        t.sport = ntohs(t.sport);
        t.dport = ntohs(t.dport);
    }

    log_debug("kprobe/ip_send_skb: pid_tgid: %d, size: %d\n", pid_tgid, size);
    handle_message(&t, size, 0);
    increment_telemetry_count(udp_send_processed);

    return 0;
}

// We can only get the accurate number of copied bytes from the return value, so we pass our
// sock* pointer from the kprobe to the kretprobe via a map (udp_recv_sock) to get all required info
//
// The same issue exists for TCP, but we can conveniently use the downstream function tcp_cleanup_rbuf
//
// On UDP side, no similar function exists in all kernel versions, though we may be able to use something like
// skb_consume_udp (v4.10+, https://elixir.bootlin.com/linux/v4.10/source/net/ipv4/udp.c#L1500)
SEC("kprobe/udp_recvmsg")
int kprobe__udp_recvmsg(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Store pointer to the socket using the pid/tgid
    bpf_map_update_elem(&udp_recv_sock, &pid_tgid, &sk, BPF_ANY);
    log_debug("kprobe/udp_recvmsg: pid_tgid: %d\n", pid_tgid);

    return 0;
}

SEC("kprobe/udp_recvmsg/pre_4_1_0")
int kprobe__udp_recvmsg_pre_4_1_0(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM2(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Store pointer to the socket using the pid/tgid
    bpf_map_update_elem(&udp_recv_sock, &pid_tgid, &sk, BPF_ANY);
    log_debug("kprobe/udp_recvmsg/pre_4_1_0: pid_tgid: %d\n", pid_tgid);

    return 0;
}

SEC("kretprobe/udp_recvmsg")
int kretprobe__udp_recvmsg(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Retrieve socket pointer from kprobe via pid/tgid
    struct sock** skpp = bpf_map_lookup_elem(&udp_recv_sock, &pid_tgid);
    if (skpp == 0) { // Missed entry
        return 0;
    }
    struct sock* sk = *skpp;

    // Make sure we clean up that pointer reference
    bpf_map_delete_elem(&udp_recv_sock, &pid_tgid);

    int copied = (int)PT_REGS_RC(ctx);
    if (copied < 0) { // Non-zero values are errors (e.g -EINVAL)
        return 0;
    }

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
        return 0;
    }

    log_debug("kretprobe/udp_recvmsg: pid_tgid: %d, return: %d\n", pid_tgid, copied);
    handle_message(&t, 0, copied);

    return 0;
}

SEC("kprobe/tcp_retransmit_skb")
int kprobe__tcp_retransmit_skb(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    log_debug("kprobe/tcp_retransmit\n");

    return handle_retransmit(sk);
}

SEC("kprobe/tcp_set_state")
int kprobe__tcp_set_state(struct pt_regs* ctx) {
    u8 state = (u8)PT_REGS_PARM2(ctx);

    // For now we're tracking only TCP_ESTABLISHED
    if (state != TCP_ESTABLISHED) {
        return 0;
    }

    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    tcp_stats_t stats = { .state_transitions = (1 << state) };
    update_tcp_stats(&t, stats);

    return 0;
}

SEC("kretprobe/inet_csk_accept")
int kretprobe__inet_csk_accept(struct pt_regs* ctx) {
    struct sock* newsk = (struct sock*)PT_REGS_RC(ctx);

    if (newsk == NULL) {
        return 0;
    }

    __u16 lport = 0;

    bpf_probe_read(&lport, sizeof(lport), ((char*)newsk) + offset_dport() + sizeof(lport));

    if (lport == 0) {
        return 0;
    }

    __u8* val = bpf_map_lookup_elem(&port_bindings, &lport);

    if (val == NULL) {
        __u8 state = PORT_LISTENING;

        bpf_map_update_elem(&port_bindings, &lport, &state, BPF_ANY);
    }

    return 0;
}

SEC("kprobe/tcp_v4_destroy_sock")
int kprobe__tcp_v4_destroy_sock(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);

    if (sk == NULL) {
        log_debug("ERR(tcp_v4_destroy_sock): socket is null \n");
        return 0;
    }

    __u16 lport = 0;

    bpf_probe_read(&lport, sizeof(lport), ((char*)sk) + offset_dport() + sizeof(lport));

    if (lport == 0) {
        log_debug("ERR(tcp_v4_destroy_sock): lport is 0 \n");
        return 0;
    }

    __u8* val = bpf_map_lookup_elem(&port_bindings, &lport);

    if (val != NULL) {
        __u8 state = PORT_CLOSED;
        bpf_map_update_elem(&port_bindings, &lport, &state, BPF_ANY);
    }

    log_debug("kprobe/tcp_v4_destroy_sock: lport: %d\n", lport);
    return 0;
}

SEC("kprobe/udp_destroy_sock")
int kprobe__udp_destroy_sock(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);

    if (sk == NULL) {
        log_debug("ERR(udp_destroy_sock): socket is null \n");
        return 0;
    }

    // get the port for the current sock
    __u16 lport = 0;
    bpf_probe_read(&lport, sizeof(lport), ((char*)sk) + offset_sport());
    lport = ntohs(lport);

    if (lport == 0) {
        log_debug("ERR(udp_destroy_sock): lport is 0 \n");
        return 0;
    }

    // decide if the port is bound, if not, do nothing
    __u8* state = bpf_map_lookup_elem(&udp_port_bindings, &lport);

    if (state == NULL) {
        log_debug("kprobe/udp_destroy_sock: sock was not listening, will drop event\n");
        return 0;
    }

    // set the state to closed
    __u8 new_state = PORT_CLOSED;
    bpf_map_update_elem(&udp_port_bindings, &lport, &new_state, BPF_ANY);

    log_debug("kprobe/udp_destroy_sock: port %d marked as closed\n", lport);

    return 0;
}

//region sys_enter_bind

static __always_inline int sys_enter_bind(__u64 fd, struct sockaddr* addr) {
    __u64 tid = bpf_get_current_pid_tgid();

    // determine if the fd for this process is an unbound UDP socket
    __u64 fd_and_tid = (tid << 32) | fd;
    __u64* u = bpf_map_lookup_elem(&unbound_sockets, &fd_and_tid);

    if (u == NULL) {
        log_debug("sys_enter_bind: bind happened, but not on a UDP socket, fd=%u, tid=%u\n", fd, tid);
        return 0;
    }

    if (addr == NULL) {
        log_debug("sys_enter_bind: could not read sockaddr, fd=%u, tid=%u\n", fd, tid);
        return 0;
    }

    // sockaddr is part of the syscall ABI, so we can hardcode the offset of 2 to find the port.
    u16 sin_port = 0;
    bpf_probe_read(&sin_port, sizeof(u16), (char*)addr + 2);
    sin_port = ntohs(sin_port);

    // write to pending_binds so the retprobe knows we can mark this as binding.
    bind_syscall_args_t args = {};
    args.fd = fd;
    args.port = sin_port;

    bpf_map_update_elem(&pending_bind, &tid, &args, BPF_ANY);
    log_debug("sys_enter_bind: started a bind on UDP port=%d fd=%u tid=%u\n", sin_port, fd, tid);

    return 0;
}

SEC("tracepoint/syscalls/sys_enter_bind")
int tracepoint__sys_enter_bind(struct syscalls_enter_bind_args* ctx) {
    log_debug("tp/sys_enter_bind: fd=%u, umyaddr=%x\n", ctx->fd, ctx->umyaddr);
    return sys_enter_bind(ctx->fd, ctx->umyaddr);
}

SEC("kprobe/sys_bind/x64")
int kprobe__sys_bind_x64(struct pt_regs* ctx) {
    struct pt_regs* _ctx = (struct pt_regs*)PT_REGS_PARM1(ctx);

    __u64 fd;
    struct sockaddr* addr;
    bpf_probe_read(&fd, sizeof(fd), &(PT_REGS_PARM1(_ctx)));
    bpf_probe_read(&addr, sizeof(struct sockaddr*), &(PT_REGS_PARM2(_ctx)));
    log_debug("kprobe/sys_bind/x64: fd=%u, umyaddr=%x\n", fd, addr);
    return sys_enter_bind(fd, addr);
}

SEC("kprobe/sys_bind")
int kprobe__sys_bind(struct pt_regs* ctx) {
    __u64 fd = PT_REGS_PARM1(ctx);
    struct sockaddr* addr = (struct sockaddr*)PT_REGS_PARM2(ctx);
    log_debug("kprobe/sys_bind: fd=%u, umyaddr=%x\n", fd, addr);
    return sys_enter_bind(fd, addr);
}

//endregion

//region sys_exit_bind

static __always_inline int sys_exit_bind(__s64 ret) {
    __u64 tid = bpf_get_current_pid_tgid();

    // bail if this bind() is not the one we're instrumenting
    bind_syscall_args_t* args;
    args = bpf_map_lookup_elem(&pending_bind, &tid);

    log_debug("sys_exit_bind: tid=%u, ret=%d\n", tid, ret);

    if (args == NULL) {
        log_debug("sys_exit_bind: was not a UDP bind, will not process\n");
        return 0;
    }

    if (ret != 0) {
        return 0;
    }

    __u16 sin_port = args->port;
    __u8 port_state = PORT_LISTENING;
    bpf_map_update_elem(&udp_port_bindings, &sin_port, &port_state, BPF_ANY);
    log_debug("sys_exit_bind: bound UDP port %u\n", sin_port);

    return 0;
}

SEC("tracepoint/syscalls/sys_exit_bind")
int tracepoint__sys_exit_bind(struct syscalls_exit_args* ctx) {
    log_debug("tp/sys_exit_bind: ret=%d\n", ctx->ret);
    return sys_exit_bind(ctx->ret);
}

SEC("kretprobe/sys_bind")
int kretprobe__sys_bind(struct pt_regs* ctx) {
    __s64 ret = PT_REGS_RC(ctx);
    log_debug("kretprobe/sys_bind: ret=%d\n", ret);
    return sys_exit_bind(ret);
}

//endregion

//region sys_enter_socket

// used for capturing UDP sockets that are bound
static __always_inline int sys_enter_socket(__u64 family, __u64 type) {
    __u64 tid = bpf_get_current_pid_tgid();
    log_debug("sys_enter_socket: tid=%u, family=%u, type=%u\n", tid, family, type);

    // figuring out if the socket being constructed is UDP. We will call
    // a socket UDP if it is in the AF_INET or AF_INET6 domain. And
    // the type is SOCK_DGRAM.
    __u8 pending_udp = 0;
    if ((family & (AF_INET | AF_INET6)) > 0 && (type & SOCK_DGRAM) > 0) {
        pending_udp = 1;
    }

    if (pending_udp == 0) {
        log_debug("sys_enter_socket: got a socket() call, but was not for UDP with tid=%u, family=%u, type=%u\n", tid, family, type);
        return 0;
    }

    log_debug("sys_enter_socket: started a UDP socket for tid=%u\n", tid);
    __u8 x = 1;
    bpf_map_update_elem(&pending_sockets, &tid, &x, BPF_ANY);

    return 0;
}

SEC("tracepoint/syscalls/sys_enter_socket")
int tracepoint__sys_enter_socket(struct syscalls_enter_socket_args* ctx) {
    log_debug("tp/sys_enter_socket: family=%u, type=%u\n", ctx->family, ctx->type);
    return sys_enter_socket(ctx->family, ctx->type);
}

SEC("kprobe/sys_socket/x64")
int kprobe__sys_socket_x64(struct pt_regs* ctx) {
    struct pt_regs* _ctx = (struct pt_regs*)PT_REGS_PARM1(ctx);

    __u64 family;
    __u64 type;
    bpf_probe_read(&family, sizeof(family), &(PT_REGS_PARM1(_ctx)));
    bpf_probe_read(&type, sizeof(type), &(PT_REGS_PARM2(_ctx)));
    log_debug("kprobe/sys_socket/x64: family=%u, type=%u\n", family, type);
    return sys_enter_socket(family, type);
}

SEC("kprobe/sys_socket")
int kprobe__sys_socket(struct pt_regs* ctx) {
    __u64 family = PT_REGS_PARM1(ctx);
    __u64 type = PT_REGS_PARM2(ctx);
    log_debug("kprobe/sys_socket: family=%u, type=%u\n", family, type);
    return sys_enter_socket(family, type);
}

//endregion

//region sys_exit_socket

// used in combination with the kprobe for sys_socket to find file descriptors for UDP sockets that have not
// yet been "binded".
static __always_inline int sys_exit_socket(__s64 fd) {
    __u64 tid = bpf_get_current_pid_tgid();
    __u8* udp_pending = bpf_map_lookup_elem(&pending_sockets, &tid);

    // move the socket to "unbound"
    __u64 fd_and_tid = (tid << 32) | fd;

    if (udp_pending == NULL) {
        // in most cases this will be a no-op, but
        // in the case that this is a non-UDP socket call,
        // and an older process with the same TID created a UDP
        // socket with the same FD, we want to prevent
        // subsequent calls to bind() from having an effect.
        bpf_map_delete_elem(&unbound_sockets, &fd_and_tid);
        log_debug("sys_exit_socket: socket() call finished but was not UDP, fd=%d, tid=%u\n", fd, tid);
        return 0;
    }

    if (fd == -1) {
        // if the socket() call has failed, don't keep track of the returned
        // file descriptor (which will be negative one)
        bpf_map_delete_elem(&unbound_sockets, &fd_and_tid);
        log_debug("sys_exit_socket: socket() call failed, fd=%d, tid=%u\n", fd, tid);
    }

    bpf_map_delete_elem(&pending_sockets, &tid);

    log_debug("sys_exit_socket: socket() call for UDP socket terminated, fd (%d) is now unbound tid=%u\n", fd, tid);

    __u64 v = 1;
    bpf_map_update_elem(&unbound_sockets, &fd_and_tid, &v, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_socket")
int tracepoint__sys_exit_socket(struct syscalls_exit_args* ctx) {
    log_debug("tp/sys_exit_socket: fd=%d\n", ctx->ret);
    return sys_exit_socket(ctx->ret);
}

SEC("kretprobe/sys_socket")
int kretprobe__sys_socket(struct pt_regs* ctx) {
    __s64 fd = PT_REGS_RC(ctx);
    log_debug("kretprobe/sys_socket: fd=%d\n", fd);
    return sys_exit_socket(fd);
}

//endregion

// This function is meant to be used as a BPF_PROG_TYPE_SOCKET_FILTER.
// When attached to a RAW_SOCKET, this code filters out everything but DNS traffic.
// All structs referenced here are kernel independent as they simply map protocol headers (Ethernet, IP and UDP).
SEC("socket/dns_filter")
int socket__dns_filter(struct __sk_buff* skb) {
    __u16 l3_proto = load_half(skb, offsetof(struct ethhdr, h_proto));
    __u8 l4_proto;
    size_t ip_hdr_size;
    size_t src_port_offset;
    size_t dst_port_offset;

    switch (l3_proto) {
    case ETH_P_IP:
        ip_hdr_size = sizeof(struct iphdr);
        l4_proto = load_byte(skb, ETH_HLEN + offsetof(struct iphdr, protocol));
        break;
    case ETH_P_IPV6:
        ip_hdr_size = sizeof(struct ipv6hdr);
        l4_proto = load_byte(skb, ETH_HLEN + offsetof(struct ipv6hdr, nexthdr));
        break;
    default:
        return 0;
    }

    switch (l4_proto) {
    case IPPROTO_UDP:
        src_port_offset = offsetof(struct udphdr, source);
        dst_port_offset = offsetof(struct udphdr, dest);
        break;
    case IPPROTO_TCP:
        src_port_offset = offsetof(struct tcphdr, source);
        dst_port_offset = offsetof(struct tcphdr, dest);
        break;
    default:
        return 0;
    }

    __u16 src_port = load_half(skb, ETH_HLEN + ip_hdr_size + src_port_offset);
    __u16 dst_port = load_half(skb, ETH_HLEN + ip_hdr_size + dst_port_offset);

    if (src_port != 53 && (!dns_stats_enabled() || dst_port != 53))
        return 0;

    return -1;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
