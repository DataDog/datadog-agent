#include <linux/kconfig.h>

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wgnu-variable-sized-type-not-at-end"
#pragma clang diagnostic ignored "-Waddress-of-packed-member"
#include <linux/ptrace.h>
#pragma clang diagnostic pop
#include "bpf_helpers.h"
#include "tracer-ebpf.h"
#include <linux/version.h>

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wtautological-compare"
#pragma clang diagnostic ignored "-Wgnu-variable-sized-type-not-at-end"
#pragma clang diagnostic ignored "-Wenum-conversion"
#include <net/sock.h>
#pragma clang diagnostic pop
#include <net/inet_sock.h>
#include <net/net_namespace.h>

/* Macro to output debug logs to /sys/kernel/debug/tracing/trace_pipe
 */
#if DEBUG == 1
#define log_debug(fmt, ...)                                        \
    ({                                                             \
        char ____fmt[] = fmt;                                      \
        bpf_trace_printk(____fmt, sizeof(____fmt), ##__VA_ARGS__); \
    })
#else
// No op
#define log_debug(fmt, ...)
#endif

/* Macro to execute the given expression replacing family by the correct family
 */
#define handle_family(sk, status, expr)                                             \
    ({                                                                              \
        if (check_family(sk, status, AF_INET)) {                                    \
            if (!are_offsets_ready_v4(status, sk)) {                                \
                return 0;                                                           \
            }                                                                       \
            metadata_mask_t family = CONN_V4;                                       \
            expr;                                                                   \
        } else if (is_ipv6_enabled(status) && check_family(sk, status, AF_INET6)) { \
            if (!are_offsets_ready_v6(status, sk)) {                                \
                return 0;                                                           \
            }                                                                       \
            metadata_mask_t family = CONN_V6;                                       \
            expr;                                                                   \
        }                                                                           \
    })

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
struct bpf_map_def SEC("maps/tcp_close_events") tcp_close_event = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 0, // This will get overridden at runtime
    .pinning = 0,
    .namespace = "",
};

/* These maps are used to match the kprobe & kretprobe of connect for IPv4 */
/* This is a key/value store with the keys being a pid
 * and the values being a struct sock *.
 */
struct bpf_map_def SEC("maps/connectsock_ipv4") connectsock_ipv4 = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(void*),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

/* These maps are used to match the kprobe & kretprobe of connect for IPv6 */
/* This is a key/value store with the keys being a pid
 * and the values being a struct sock *.
 */
struct bpf_map_def SEC("maps/connectsock_ipv6") connectsock_ipv6 = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(void*),
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

/* This maps tracks listening ports. Entries are added to the map via tracing the inet_csk_accept syscall.  The
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

/* This maps is used for telemetry in kernelspace
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
__attribute__((always_inline))
static bool is_big_endian(void) {
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
__attribute__((always_inline))
static bool is_ipv4_mapped_ipv6(u64 saddr_h, u64 saddr_l, u64 daddr_h, u64 daddr_l) {
    if (is_big_endian()) {
        return ((saddr_h == 0 && ((u32)(saddr_l >> 32) == 0x0000FFFF)) || (daddr_h == 0 && ((u32)(daddr_l >> 32) == 0x0000FFFF)));
    } else {
        return ((saddr_h == 0 && ((u32)saddr_l == 0xFFFF0000)) || (daddr_h == 0 && ((u32)daddr_l == 0xFFFF0000)));
    }
}

struct bpf_map_def SEC("maps/tracer_status") tracer_status = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(tracer_status_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

// Keeping track of latest timestamp of monotonic clock
struct bpf_map_def SEC("maps/latest_ts") latest_ts = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(__u64),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

__attribute__((always_inline))
static bool proc_t_comm_equals(proc_t a, proc_t b) {
    int i;
    for (i = 0; i < TASK_COMM_LEN; i++) {
        if (a.comm[i] != b.comm[i]) {
            return false;
        }
    }
    return true;
}

__attribute__((always_inline))
static int are_offsets_ready_v4(tracer_status_t* status, struct sock* skp) {
    u64 zero = 0;

    switch (status->state) {
    case TRACER_STATE_UNINITIALIZED:
        return 0;
    case TRACER_STATE_CHECKING:
        break;
    case TRACER_STATE_CHECKED:
        return 0;
    case TRACER_STATE_READY:
        return 1;
    default:
        return 0;
    }

    // Only traffic for the expected process name. Extraneous connections from other processes must be ignored here.
    // Userland must take care to generate connections from the correct thread. In Golang, this can be achieved
    // with runtime.LockOSThread.
    proc_t proc = {};
    bpf_get_current_comm(&proc.comm, sizeof(proc.comm));

    if (!proc_t_comm_equals(status->proc, proc))
        return 0;

    tracer_status_t new_status = {};
    new_status.state = TRACER_STATE_CHECKED;
    new_status.what = status->what;
    new_status.offset_saddr = status->offset_saddr;
    new_status.offset_daddr = status->offset_daddr;
    new_status.offset_sport = status->offset_sport;
    new_status.offset_dport = status->offset_dport;
    new_status.offset_netns = status->offset_netns;
    new_status.offset_ino = status->offset_ino;
    new_status.offset_family = status->offset_family;
    new_status.offset_daddr_ipv6 = status->offset_daddr_ipv6;
    new_status.err = 0;
    new_status.saddr = status->saddr;
    new_status.daddr = status->daddr;
    new_status.sport = status->sport;
    new_status.dport = status->dport;
    new_status.netns = status->netns;
    new_status.family = status->family;
    new_status.ipv6_enabled = status->ipv6_enabled;

    bpf_probe_read(&new_status.proc.comm, sizeof(proc.comm), proc.comm);

    int i;
    for (i = 0; i < 4; i++) {
        new_status.daddr_ipv6[i] = status->daddr_ipv6[i];
    }

    u32 possible_saddr;
    u32 possible_daddr;
    u16 possible_sport;
    u16 possible_dport;
    possible_net_t* possible_skc_net;
    u32 possible_netns;
    u16 possible_family;
    long ret = 0;

    switch (status->what) {
    case GUESS_SADDR:
        possible_saddr = 0;
        bpf_probe_read(&possible_saddr, sizeof(possible_saddr), ((char*)skp) + status->offset_saddr);
        new_status.saddr = possible_saddr;
        break;
    case GUESS_DADDR:
        possible_daddr = 0;
        bpf_probe_read(&possible_daddr, sizeof(possible_daddr), ((char*)skp) + status->offset_daddr);
        new_status.daddr = possible_daddr;
        break;
    case GUESS_FAMILY:
        possible_family = 0;
        bpf_probe_read(&possible_family, sizeof(possible_family), ((char*)skp) + status->offset_family);
        new_status.family = possible_family;
        break;
    case GUESS_SPORT:
        possible_sport = 0;
        bpf_probe_read(&possible_sport, sizeof(possible_sport), ((char*)skp) + status->offset_sport);
        new_status.sport = possible_sport;
        break;
    case GUESS_DPORT:
        possible_dport = 0;
        bpf_probe_read(&possible_dport, sizeof(possible_dport), ((char*)skp) + status->offset_dport);
        new_status.dport = possible_dport;
        break;
    case GUESS_NETNS:
        possible_netns = 0;
        possible_skc_net = NULL;
        bpf_probe_read(&possible_skc_net, sizeof(possible_net_t*), ((char*)skp) + status->offset_netns);
        // if we get a kernel fault, it means possible_skc_net
        // is an invalid pointer, signal an error so we can go
        // to the next offset_netns
        ret = bpf_probe_read(&possible_netns, sizeof(possible_netns), ((char*)possible_skc_net) + status->offset_ino);
        if (ret == -EFAULT) {
            new_status.err = 1;
            break;
        }
        new_status.netns = possible_netns;
        break;
    default:
        // not for us
        return 0;
    }

    bpf_map_update_elem(&tracer_status, &zero, &new_status, BPF_ANY);

    return 0;
}

__attribute__((always_inline))
static int are_offsets_ready_v6(tracer_status_t* status, struct sock* skp) {
    u64 zero = 0;

    switch (status->state) {
    case TRACER_STATE_UNINITIALIZED:
        return 0;
    case TRACER_STATE_CHECKING:
        break;
    case TRACER_STATE_CHECKED:
        return 0;
    case TRACER_STATE_READY:
        return 1;
    default:
        return 0;
    }

    // Only traffic for the expected process name. Extraneous connections from other processes must be ignored here.
    // Userland must take care to generate connections from the correct thread. In Golang, this can be achieved
    // with runtime.LockOSThread.
    proc_t proc = {};
    bpf_get_current_comm(&proc.comm, sizeof(proc.comm));

    if (!proc_t_comm_equals(status->proc, proc))
        return 0;

    tracer_status_t new_status = {};
    new_status.state = TRACER_STATE_CHECKED;
    new_status.what = status->what;
    new_status.offset_saddr = status->offset_saddr;
    new_status.offset_daddr = status->offset_daddr;
    new_status.offset_sport = status->offset_sport;
    new_status.offset_dport = status->offset_dport;
    new_status.offset_netns = status->offset_netns;
    new_status.offset_ino = status->offset_ino;
    new_status.offset_family = status->offset_family;
    new_status.offset_daddr_ipv6 = status->offset_daddr_ipv6;
    new_status.err = 0;
    new_status.saddr = status->saddr;
    new_status.daddr = status->daddr;
    new_status.sport = status->sport;
    new_status.dport = status->dport;
    new_status.netns = status->netns;
    new_status.family = status->family;
    new_status.ipv6_enabled = status->ipv6_enabled;

    bpf_probe_read(&new_status.proc.comm, sizeof(proc.comm), proc.comm);

    int i;
    for (i = 0; i < 4; i++) {
        new_status.daddr_ipv6[i] = status->daddr_ipv6[i];
    }

    u32 possible_daddr_ipv6[4] = {};
    switch (status->what) {
    case GUESS_DADDR_IPV6:
        bpf_probe_read(&possible_daddr_ipv6, sizeof(possible_daddr_ipv6), ((char*)skp) + status->offset_daddr_ipv6);

        int i;
        for (i = 0; i < 4; i++) {
            new_status.daddr_ipv6[i] = possible_daddr_ipv6[i];
        }
        break;
    default:
        // not for us
        return 0;
    }

    bpf_map_update_elem(&tracer_status, &zero, &new_status, BPF_ANY);

    return 0;
}

__attribute__((always_inline))
static bool check_family(struct sock* sk, tracer_status_t* status, u16 expected_family) {
    u16 family = 0;
    bpf_probe_read(&family, sizeof(u16), ((char*)sk) + status->offset_family);
    return family == expected_family;
}

__attribute__((always_inline))
static bool is_ipv6_enabled(tracer_status_t* status) {
    return status->ipv6_enabled == TRACER_IPV6_ENABLED;
}

__attribute__((always_inline))
static int read_conn_tuple(conn_tuple_t* tuple, tracer_status_t* status, struct sock* skp, metadata_mask_t type, metadata_mask_t family) {
    u32 net_ns_inum;
    u16 sport, dport;
    u64 saddr_h, saddr_l, daddr_h, daddr_l;
    possible_net_t* skc_net;

    saddr_h = 0;
    saddr_l = 0;
    daddr_h = 0;
    daddr_l = 0;
    sport = 0;
    dport = 0;
    skc_net = NULL;
    net_ns_inum = 0;

    // Retrieve addresses
    if (family == CONN_V4) {
        bpf_probe_read(&saddr_l, sizeof(u32), ((char*)skp) + status->offset_saddr);
        bpf_probe_read(&daddr_l, sizeof(u32), ((char*)skp) + status->offset_daddr);

        if (!saddr_l || !daddr_l) {
            log_debug("ERR(read_conn_tuple.v4): src/dst addr not set src:%d,dst:%d\n", saddr_l, daddr_l);
            return 0;
        }
    } else {
        bpf_probe_read(&saddr_h, sizeof(saddr_h), ((char*)skp) + status->offset_daddr_ipv6 + 2 * sizeof(u64));
        bpf_probe_read(&saddr_l, sizeof(saddr_l), ((char*)skp) + status->offset_daddr_ipv6 + 3 * sizeof(u64));
        bpf_probe_read(&daddr_h, sizeof(daddr_h), ((char*)skp) + status->offset_daddr_ipv6);
        bpf_probe_read(&daddr_l, sizeof(daddr_l), ((char*)skp) + status->offset_daddr_ipv6 + sizeof(u64));

        // We can only pass 4 args to bpf_trace_printk
        // so split those 2 statements to be able to log everything
        if (!(saddr_h || saddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): src addr not set: src_l:%d,src_h:%d\n",
                saddr_l, saddr_h);
            return 0;
        }

        if (!(daddr_h || daddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): dst addr not set: dst_l:%d,dst_h:%d\n",
                daddr_l, daddr_h);
            return 0;
        }
    }

    // Retrieve ports
    bpf_probe_read(&sport, sizeof(sport), ((char*)skp) + status->offset_sport);
    bpf_probe_read(&dport, sizeof(dport), ((char*)skp) + status->offset_dport);

    if (sport == 0 || dport == 0) {
        log_debug("ERR(read_conn_tuple.v4): src/dst port not set: src:%d, dst:%d\n", sport, dport);
        return 0;
    }

    // Retrieve network namespace id
    bpf_probe_read(&skc_net, sizeof(void*), ((char*)skp) + status->offset_netns);
    bpf_probe_read(&net_ns_inum, sizeof(net_ns_inum), ((char*)skc_net) + status->offset_ino);

    tuple->saddr_h = saddr_h;
    tuple->saddr_l = saddr_l;
    tuple->daddr_h = daddr_h;
    tuple->daddr_l = daddr_l;
    tuple->sport = sport;
    tuple->dport = dport;
    tuple->netns = net_ns_inum;
    tuple->metadata = type;

    // Check if we can map IPv6 to IPv4
    if (family == CONN_V6 && is_ipv4_mapped_ipv6(saddr_h, saddr_l, daddr_h, daddr_l)) {
        tuple->metadata |= CONN_V4;

        tuple->saddr_h = 0;
        tuple->daddr_h = 0;
        tuple->saddr_l = (u32)(saddr_l >> 32);
        tuple->daddr_l = (u32)(daddr_l >> 32);
    } else {
        tuple->metadata |= family;
    }

    return 1;
}

__attribute__((always_inline))
static void update_conn_stats(
    struct sock* sk,
    tracer_status_t* status,
    u64 pid,
    metadata_mask_t type,
    metadata_mask_t family,
    size_t sent_bytes,
    size_t recv_bytes,
    u64 ts) {
    conn_tuple_t t = {};
    conn_stats_ts_t* val;

    if (!read_conn_tuple(&t, status, sk, type, family)) {
        return;
    }

    t.pid = pid >> 32;
    t.sport = ntohs(t.sport); // Making ports human-readable
    t.dport = ntohs(t.dport);

    // initialize-if-no-exist the connection stat, and load it
    conn_stats_ts_t empty = {};
    bpf_map_update_elem(&conn_stats, &t, &empty, BPF_NOEXIST);
    val = bpf_map_lookup_elem(&conn_stats, &t);

    // If already in our map, increment size in-place
    if (val != NULL) {
        __sync_fetch_and_add(&val->sent_bytes, sent_bytes);
        __sync_fetch_and_add(&val->recv_bytes, recv_bytes);
        val->timestamp = ts;
    }
}

__attribute__((always_inline))
static void update_tcp_stats(
    struct sock* sk,
    tracer_status_t* status,
    metadata_mask_t family,
    u32 retransmits,
    u64 ts) {
    conn_tuple_t t = {};
    tcp_stats_t* val;

    if (!read_conn_tuple(&t, status, sk, CONN_TYPE_TCP, family)) {
        return;
    }

    t.sport = ntohs(t.sport); // Making ports human-readable
    t.dport = ntohs(t.dport);

    // initialize-if-no-exist the connetion state, and load it
    tcp_stats_t empty = {};
    bpf_map_update_elem(&tcp_stats, &t, &empty, BPF_NOEXIST);
    val = bpf_map_lookup_elem(&tcp_stats, &t);
    if (val != NULL) {
        __sync_fetch_and_add(&val->retransmits, retransmits);
    }
}

__attribute__((always_inline))
static void cleanup_tcp_conn(
    struct pt_regs* ctx,
    struct sock* sk,
    tracer_status_t* status,
    u64 pid,
    metadata_mask_t family) {
    u32 cpu = bpf_get_smp_processor_id();

    // Will hold the full connection data to send through the perf buffer
    tcp_conn_t t = {
        .tup = (conn_tuple_t) {
            .pid = 0,
        },
    };
    tcp_stats_t* tst;
    conn_stats_ts_t* cst;

    if (!read_conn_tuple(&(t.tup), status, sk, CONN_TYPE_TCP, family)) {
        return;
    }

    t.tup.sport = ntohs(t.tup.sport); // Making ports human-readable
    t.tup.dport = ntohs(t.tup.dport);

    tst = bpf_map_lookup_elem(&tcp_stats, &(t.tup));
    // Delete the connection from the tcp_stats map before setting the PID
    bpf_map_delete_elem(&tcp_stats, &(t.tup));

    t.tup.pid = pid >> 32;

    cst = bpf_map_lookup_elem(&conn_stats, &(t.tup));
    // Delete this connection from our stats map
    bpf_map_delete_elem(&conn_stats, &(t.tup));

    if (tst != NULL) {
        t.tcp_stats = *tst;
    }

    if (cst != NULL) {
        cst->timestamp = bpf_ktime_get_ns();
        t.conn_stats = *cst;
    }

    // Send the connection data to the perf buffer
    bpf_perf_event_output(ctx, &tcp_close_event, cpu, &t, sizeof(t));
}

__attribute__((always_inline))
static int handle_message(struct sock* sk,
    tracer_status_t* status,
    u64 pid_tgid,
    metadata_mask_t type,
    size_t sent_bytes,
    size_t recv_bytes) {

    u64 zero = 0;
    u64 ts = bpf_ktime_get_ns();

    handle_family(sk, status, update_conn_stats(sk, status, pid_tgid, type, family, sent_bytes, recv_bytes, ts));

    // Update latest timestamp that we've seen - for connection expiration tracking
    bpf_map_update_elem(&latest_ts, &zero, &ts, BPF_ANY);
    return 0;
}

__attribute__((always_inline))
static int handle_retransmit(struct sock* sk, tracer_status_t* status) {
    u64 ts = bpf_ktime_get_ns();

    handle_family(sk, status, update_tcp_stats(sk, status, family, 1, ts));

    // Update latest timestamp that we've seen - for connection expiration tracking
    u64 zero = 0;
    bpf_map_update_elem(&latest_ts, &zero, &ts, BPF_ANY);
    return 0;
}

// Used for offset guessing (see: pkg/offsetguess.go)
SEC("kprobe/tcp_v4_connect")
int kprobe__tcp_v4_connect(struct pt_regs* ctx) {
    struct sock* sk;
    u64 pid = bpf_get_current_pid_tgid();

    sk = (struct sock*)PT_REGS_PARM1(ctx);

    bpf_map_update_elem(&connectsock_ipv4, &pid, &sk, BPF_ANY);

    return 0;
}

// Used for offset guessing (see: pkg/offsetguess.go)
SEC("kretprobe/tcp_v4_connect")
int kretprobe__tcp_v4_connect(struct pt_regs* ctx) {
    int ret = PT_REGS_RC(ctx);
    u64 pid = bpf_get_current_pid_tgid();
    struct sock** skpp;
    u64 zero = 0;
    tracer_status_t* status;

    skpp = bpf_map_lookup_elem(&connectsock_ipv4, &pid);
    if (skpp == 0) {
        return 0; // missed entry
    }

    struct sock* skp = *skpp;

    bpf_map_delete_elem(&connectsock_ipv4, &pid);

    if (ret != 0) {
        // failed to send SYNC packet, may not have populated
        // socket __sk_common.{skc_rcv_saddr, ...}
        return 0;
    }

    status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state == TRACER_STATE_UNINITIALIZED) {
        return 0;
    }

    // We should figure out offsets if they're not already figured out
    are_offsets_ready_v4(status, skp);

    return 0;
}

// Used for offset guessing (see: pkg/offsetguess.go)
SEC("kprobe/tcp_v6_connect")
int kprobe__tcp_v6_connect(struct pt_regs* ctx) {
    struct sock* sk;
    u64 pid = bpf_get_current_pid_tgid();

    sk = (struct sock*)PT_REGS_PARM1(ctx);

    bpf_map_update_elem(&connectsock_ipv6, &pid, &sk, BPF_ANY);

    return 0;
}

// Used for offset guessing (see: pkg/offsetguess.go)
SEC("kretprobe/tcp_v6_connect")
int kretprobe__tcp_v6_connect(struct pt_regs* ctx) {
    u64 pid = bpf_get_current_pid_tgid();
    u64 zero = 0;
    struct sock** skpp;
    tracer_status_t* status;
    skpp = bpf_map_lookup_elem(&connectsock_ipv6, &pid);
    if (skpp == 0) {
        return 0; // missed entry
    }

    bpf_map_delete_elem(&connectsock_ipv6, &pid);

    struct sock* skp = *skpp;

    status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state == TRACER_STATE_UNINITIALIZED) {
        return 0;
    }

    // We should figure out offsets if they're not already figured out
    are_offsets_ready_v6(status, skp);

    return 0;
}

SEC("kprobe/tcp_sendmsg")
int kprobe__tcp_sendmsg(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    size_t size = (size_t)PT_REGS_PARM3(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 zero = 0;

    tracer_status_t* status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state == TRACER_STATE_UNINITIALIZED) {
        return 0;
    }
    log_debug("kprobe/tcp_sendmsg: pid_tgid: %d, size: %d\n", pid_tgid, size);

    return handle_message(sk, status, pid_tgid, CONN_TYPE_TCP, size, 0);
}

SEC("kretprobe/tcp_sendmsg")
int kretprobe__tcp_sendmsg(struct pt_regs* ctx) {
    int ret = PT_REGS_RC(ctx);

    log_debug("kretprobe/tcp_sendmsg: return: %d\n", ret);
    // If ret < 0 it means an error occurred but we still counted the bytes as being sent
    // let's increment our miscount count
    if (ret < 0) {
        // Initialize the counter if it does not exist
        __u64 key = 0;
        telemetry_t empty = {};
        telemetry_t* val;
        bpf_map_update_elem(&telemetry, &key, &empty, BPF_NOEXIST);
        val = bpf_map_lookup_elem(&telemetry, &key);
        if (val != NULL) {
            __sync_fetch_and_add(&val->tcp_sent_miscounts, 1);
        }
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
    u64 zero = 0;

    tracer_status_t* status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state == TRACER_STATE_UNINITIALIZED) {
        return 0;
    }

    log_debug("kprobe/tcp_cleanup_rbuf: pid_tgid: %d, copied: %d\n", pid_tgid, copied);

    return handle_message(sk, status, pid_tgid, CONN_TYPE_TCP, 0, copied);
}

SEC("kprobe/tcp_close")
int kprobe__tcp_close(struct pt_regs* ctx) {
    struct sock* sk;
    tracer_status_t* status;
    u64 zero = 0;
    u64 pid_tgid = bpf_get_current_pid_tgid();
    sk = (struct sock*)PT_REGS_PARM1(ctx);

    status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state != TRACER_STATE_READY) {
        return 0;
    }

    u32 net_ns_inum;

    // Get network namespace id
    possible_net_t* skc_net;

    skc_net = NULL;
    net_ns_inum = 0;
    bpf_probe_read(&skc_net, sizeof(possible_net_t*), ((char*)sk) + status->offset_netns);
    bpf_probe_read(&net_ns_inum, sizeof(net_ns_inum), ((char*)skc_net) + status->offset_ino);

    log_debug("kprobe/tcp_close: pid_tgid: %d, ns: %d\n", pid_tgid, net_ns_inum);

    handle_family(sk, status, cleanup_tcp_conn(ctx, sk, status, pid_tgid, family));
    return 0;
}

SEC("kprobe/udp_sendmsg")
int kprobe__udp_sendmsg(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    size_t size = (size_t)PT_REGS_PARM3(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 zero = 0;

    tracer_status_t* status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state == TRACER_STATE_UNINITIALIZED) {
        return 0;
    }

    log_debug("kprobe/udp_sendmsg: pid_tgid: %d, size: %d\n", pid_tgid, size);
    handle_message(sk, status, pid_tgid, CONN_TYPE_UDP, size, 0);

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

SEC("kretprobe/udp_recvmsg")
int kretprobe__udp_recvmsg(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 zero = 0;

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

    tracer_status_t* status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state == TRACER_STATE_UNINITIALIZED) {
        return 0;
    }

    handle_message(sk, status, pid_tgid, CONN_TYPE_UDP, 0, copied);

    return 0;
}

SEC("kprobe/tcp_retransmit_skb")
int kprobe__tcp_retransmit_skb(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    u64 zero = 0;
    tracer_status_t* status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state == TRACER_STATE_UNINITIALIZED) {
        return 0;
    }
    log_debug("kprobe/tcp_retransmit\n");

    return handle_retransmit(sk, status);
}

SEC("kretprobe/inet_csk_accept")
int kretprobe__inet_csk_accept(struct pt_regs* ctx) {
    struct sock* newsk = (struct sock*)PT_REGS_RC(ctx);

    if (newsk == NULL) {
        return 0;
    }

    u64 zero = 0;
    tracer_status_t* status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state != TRACER_STATE_READY) {
        return 0;
    }

    __u16 lport = 0;

    bpf_probe_read(&lport, sizeof(lport), ((char*)newsk) + status->offset_dport + sizeof(lport));

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

    u64 zero = 0;
    tracer_status_t* status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL || status->state != TRACER_STATE_READY) {
        return 0;
    }

    __u16 lport = 0;

    bpf_probe_read(&lport, sizeof(lport), ((char*)sk) + status->offset_dport + sizeof(lport));

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

// This number will be interpreted by gobpf-elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE;

char _license[] SEC("license") = "GPL";
