#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "compiler.h"
#include <asm-generic/errno-base.h>
#include "map-defs.h"
#include "cgroup.h"
#include "socket-contention-kern-user.h"
#include "bpf_metadata.h"
#include "bpf_telemetry.h"

#define SOCKET_CONTENTION_MAX_INFLIGHT     1024
#define SOCKET_CONTENTION_MAX_LOCKS        32768
#define SOCKET_CONTENTION_MAX_AGGREGATIONS 4096

struct tstamp_data {
    __u64 timestamp;
    __u64 lock;
    __u32 flags;
};

BPF_HASH_MAP(tstamp, __u32, struct tstamp_data, SOCKET_CONTENTION_MAX_INFLIGHT)
BPF_PERCPU_ARRAY_MAP(tstamp_cpu, struct tstamp_data, 1)
BPF_HASH_MAP(socket_lock_identities, __u64, struct socket_lock_identity, SOCKET_CONTENTION_MAX_LOCKS)
BPF_PERCPU_HASH_MAP(socket_contention_stats, struct socket_contention_key, struct socket_contention_stats, SOCKET_CONTENTION_MAX_AGGREGATIONS)

/* lock contention flags from include/trace/events/lock.h */
#define LCB_F_SPIN  (1U << 0)
#define LCB_F_READ  (1U << 1)
#define LCB_F_WRITE (1U << 2)

/*
 * Returns the scratch timestamp slot for the current contention event.
 * Spin/rw locks stay on-CPU, so we can use a single per-CPU entry; sleeping
 * locks may resume on a different CPU, so they need per-task state instead.
 */
static __always_inline struct tstamp_data *get_tstamp_elem(__u32 flags)
{
    __u32 pid;
    struct tstamp_data *pelem;

    /* Use per-cpu array map for spinlock and rwlock */
    if (flags == (LCB_F_SPIN | LCB_F_READ) || flags == LCB_F_SPIN ||
        flags == (LCB_F_SPIN | LCB_F_WRITE)) {
        __u32 idx = 0;

        pelem = bpf_map_lookup_elem(&tstamp_cpu, &idx);
        if (pelem && pelem->lock)
            pelem = NULL;
        return pelem;
    }

    pid = bpf_get_current_pid_tgid();
    pelem = bpf_map_lookup_elem(&tstamp, &pid);
    if (pelem && pelem->lock)
        return NULL;

    if (!pelem) {
        struct tstamp_data zero = {};

        if (bpf_map_update_with_telemetry(tstamp, &pid, &zero, BPF_NOEXIST, -EEXIST) < 0)
            return NULL;

        pelem = bpf_map_lookup_elem(&tstamp, &pid);
        if (!pelem)
            return NULL;
    }

    return pelem;
}

static __always_inline void register_lock_identity(__u64 lock_addr, struct socket_lock_identity *identity)
{
    if (!lock_addr) {
        return;
    }

    bpf_map_update_with_telemetry(socket_lock_identities, &lock_addr, identity, BPF_ANY);
}

static __always_inline void unregister_lock_identity(__u64 lock_addr)
{
    if (!lock_addr) {
        return;
    }

    bpf_map_delete_elem(&socket_lock_identities, &lock_addr);
}

static __always_inline void register_socket_identity(struct sock *sk)
{
    struct socket_lock_identity identity = {};
    __u64 sock_ptr = (__u64)sk;

    if (!sock_ptr) {
        return;
    }

    identity.sock_ptr = sock_ptr;
    BPF_CORE_READ_INTO(&identity.socket_cookie, sk, __sk_common.skc_cookie.counter);
    identity.cgroup_id = bpf_get_current_cgroup_id();
    BPF_CORE_READ_INTO(&identity.family, sk, __sk_common.skc_family);
    BPF_CORE_READ_INTO(&identity.protocol, sk, sk_protocol);
    BPF_CORE_READ_INTO(&identity.socket_type, sk, sk_type);

    identity.lock_subtype = SOCKET_CONTENTION_LOCK_SUBTYPE_SK_LOCK;
    register_lock_identity((__u64)&sk->sk_lock.slock, &identity);

    identity.lock_subtype = SOCKET_CONTENTION_LOCK_SUBTYPE_SK_WAIT_QUEUE;
    register_lock_identity((__u64)&sk->sk_lock.wq.lock, &identity);

    identity.lock_subtype = SOCKET_CONTENTION_LOCK_SUBTYPE_CALLBACK_LOCK;
    register_lock_identity((__u64)&sk->sk_callback_lock.raw_lock, &identity);

    identity.lock_subtype = SOCKET_CONTENTION_LOCK_SUBTYPE_ERROR_QUEUE_LOCK;
    register_lock_identity((__u64)&sk->sk_error_queue.lock, &identity);

    identity.lock_subtype = SOCKET_CONTENTION_LOCK_SUBTYPE_RECEIVE_QUEUE_LOCK;
    register_lock_identity((__u64)&sk->sk_receive_queue.lock, &identity);

    identity.lock_subtype = SOCKET_CONTENTION_LOCK_SUBTYPE_WRITE_QUEUE_LOCK;
    register_lock_identity((__u64)&sk->sk_write_queue.lock, &identity);
}

static __always_inline void unregister_socket_identity(struct sock *sk)
{
    __u64 sock_ptr = (__u64)sk;

    if (!sock_ptr) {
        return;
    }

    unregister_lock_identity((__u64)&sk->sk_lock.slock);
    unregister_lock_identity((__u64)&sk->sk_lock.wq.lock);
    unregister_lock_identity((__u64)&sk->sk_callback_lock.raw_lock);
    unregister_lock_identity((__u64)&sk->sk_error_queue.lock);
    unregister_lock_identity((__u64)&sk->sk_receive_queue.lock);
    unregister_lock_identity((__u64)&sk->sk_write_queue.lock);
}

static __always_inline struct socket_contention_stats *get_or_create_stats(struct socket_contention_key *key)
{
    struct socket_contention_stats *stats = bpf_map_lookup_elem(&socket_contention_stats, key);

    if (!stats) {
        struct socket_contention_stats zero = {};

        bpf_map_update_with_telemetry(socket_contention_stats, key, &zero, BPF_NOEXIST, -EEXIST);
        stats = bpf_map_lookup_elem(&socket_contention_stats, key);
    }

    return stats;
}

static __always_inline void update_stats(struct socket_contention_key *key, __u64 duration)
{
    struct socket_contention_stats *stats = get_or_create_stats(key);
    __u64 prev_count;

    if (!stats) {
        return;
    }

    __sync_fetch_and_add(&stats->total_time_ns, duration);
    prev_count = __sync_fetch_and_add(&stats->count, 1);

    if (prev_count == 0 || stats->min_time_ns > duration) {
        stats->min_time_ns = duration;
    }
    if (stats->max_time_ns < duration) {
        stats->max_time_ns = duration;
    }
}

SEC("kprobe/sock_init_data")
int BPF_KPROBE(kprobe__sock_init_data, struct socket *sock, struct sock *sk)
{
    register_socket_identity(sk);
    return 0;
}

SEC("kprobe/tcp_connect")
int BPF_KPROBE(kprobe__tcp_connect, struct sock *sk)
{
    register_socket_identity(sk);
    return 0;
}

SEC("kretprobe/inet_csk_accept")
int BPF_KRETPROBE(kretprobe__inet_csk_accept, struct sock *sk)
{
    register_socket_identity(sk);
    return 0;
}

SEC("kprobe/tcp_close")
int BPF_KPROBE(kprobe__tcp_close, struct sock *sk)
{
    unregister_socket_identity(sk);
    return 0;
}

SEC("kprobe/inet_csk_listen_stop")
int BPF_KPROBE(kprobe__inet_csk_listen_stop, struct sock *sk)
{
    unregister_socket_identity(sk);
    return 0;
}

SEC("kprobe/__sk_destruct")
int BPF_KPROBE(kprobe____sk_destruct, struct sock *sk)
{
    unregister_socket_identity(sk);
    return 0;
}

SEC("tp_btf/contention_begin")
int tp_contention_begin(__u64 *ctx)
{
    struct tstamp_data *pelem;

    /* contention_begin passes the contended lock pointer in ctx[0] and the lock flags in ctx[1]. */
    pelem = get_tstamp_elem((__u32)ctx[1]);
    if (!pelem)
        return 0;

    pelem->timestamp = bpf_ktime_get_ns();
    pelem->lock = ctx[0];
    pelem->flags = (__u32)ctx[1];
    return 0;
}

SEC("tp_btf/contention_end")
int tp_contention_end(__u64 *ctx)
{
    __u32 pid = 0, idx = 0;
    struct tstamp_data *pelem;
    struct socket_lock_identity *identity;
    struct socket_contention_key key = {};
    __u64 duration;
    bool need_delete = false;

    pelem = bpf_map_lookup_elem(&tstamp_cpu, &idx);
    if (pelem && pelem->lock) {
        if (pelem->lock != ctx[0])
            return 0;
    } else {
        pid = bpf_get_current_pid_tgid();
        pelem = bpf_map_lookup_elem(&tstamp, &pid);
        if (!pelem || pelem->lock != ctx[0])
            return 0;
        need_delete = true;
    }

    duration = bpf_ktime_get_ns() - pelem->timestamp;
    if ((__s64)duration < 0) {
        goto cleanup;
    }

    identity = bpf_map_lookup_elem(&socket_lock_identities, &pelem->lock);
    if (!identity) {
        goto cleanup;
    }

    key.flags = pelem->flags;
    key.object_kind = SOCKET_CONTENTION_OBJECT_KIND_SOCKET;
    key.socket_type = identity->socket_type;
    key.family = identity->family;
    key.protocol = identity->protocol;
    key.lock_subtype = identity->lock_subtype;
    key.cgroup_id = identity->cgroup_id;

    update_stats(&key, duration);

cleanup:
    pelem->lock = 0;
    if (need_delete)
        bpf_map_delete_elem(&tstamp, &pid);
    return 0;
}

char _license[] SEC("license") = "GPL";
