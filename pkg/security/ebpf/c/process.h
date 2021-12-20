#ifndef _PROCESS_H_
#define _PROCESS_H_

#include <linux/tty.h>
#include <linux/sched.h>

#include "container.h"
#include "span.h"

struct proc_cache_t {
    struct container_context_t container;
    struct file_t executable;

    u64 exec_timestamp;
    char tty_name[TTY_NAME_LEN];
    char comm[TASK_COMM_LEN];
};

static __attribute__((always_inline)) u32 copy_tty_name(const char src[TTY_NAME_LEN], char dst[TTY_NAME_LEN]) {
    if (src[0] == 0) {
        return 0;
    }

#pragma unroll
    for (int i = 0; i < TTY_NAME_LEN; i++)
    {
        dst[i] = src[i];
    }
    return TTY_NAME_LEN;
}

void __attribute__((always_inline)) copy_proc_cache_except_comm(struct proc_cache_t* src, struct proc_cache_t* dst) {
    copy_container_id(src->container.container_id, dst->container.container_id);
    dst->executable = src->executable;
    dst->exec_timestamp = src->exec_timestamp;
    copy_tty_name(src->tty_name, dst->tty_name);
}

void __attribute__((always_inline)) copy_proc_cache(struct proc_cache_t *src, struct proc_cache_t *dst) {
    copy_proc_cache_except_comm(src, dst);
    bpf_probe_read(dst->comm, TASK_COMM_LEN, src->comm);
}

struct bpf_map_def SEC("maps/proc_cache") proc_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct proc_cache_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

static void __attribute__((always_inline)) fill_container_context(struct proc_cache_t *entry, struct container_context_t *context) {
    if (entry) {
        copy_container_id(entry->container.container_id, context->container_id);
    }
}

struct credentials_t {
    u32 uid;
    u32 gid;
    u32 euid;
    u32 egid;
    u32 fsuid;
    u32 fsgid;
    u64 cap_effective;
    u64 cap_permitted;
};

void __attribute__((always_inline)) copy_credentials(struct credentials_t* src, struct credentials_t* dst) {
    *dst = *src;
}

struct pid_cache_t {
    u32 cookie;
    u32 ppid;
    u64 fork_timestamp;
    u64 exit_timestamp;
    struct credentials_t credentials;
};

void __attribute__((always_inline)) copy_pid_cache_except_exit_ts(struct pid_cache_t* src, struct pid_cache_t* dst) {
    dst->cookie = src->cookie;
    dst->ppid = src->ppid;
    dst->fork_timestamp = src->fork_timestamp;
    dst->credentials = src->credentials;
}

struct bpf_map_def SEC("maps/pid_cache") pid_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct pid_cache_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

// defined in exec.h
struct proc_cache_t *get_proc_from_cookie(u32 cookie);

struct proc_cache_t * __attribute__((always_inline)) get_proc_cache(u32 tgid) {
    struct proc_cache_t *entry = NULL;

    struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (pid_entry) {
        // Select the cache entry
        u32 cookie = pid_entry->cookie;
        entry = get_proc_from_cookie(cookie);
    }
    return entry;
}

struct bpf_map_def SEC("maps/netns_cache") netns_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 40960,
    .pinning = 0,
    .namespace = "",
};

static struct proc_cache_t * __attribute__((always_inline)) fill_process_context(struct process_context_t *data) {
    // Pid & Tid
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    // https://github.com/iovisor/bcc/blob/master/docs/reference_guide.md#4-bpf_get_current_pid_tgid
    data->pid = tgid;
    data->tid = pid_tgid;

    u32 *netns = bpf_map_lookup_elem(&netns_cache, &data->tid);
    if (netns != NULL) {
        data->netns = *netns;
    }

    return get_proc_cache(tgid);
}

struct bpf_map_def SEC("maps/root_nr_namespace_nr") root_nr_namespace_nr = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 32768,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/namespace_nr_root_nr") namespace_nr_root_nr = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 32768,
    .pinning = 0,
    .namespace = "",
};

void __attribute__((always_inline)) register_nr(u32 root_nr, u64 namespace_nr) {
    // no namespace
    if (root_nr == 0 || namespace_nr == 0) {
        return;
    }

    // TODO(will): this can conflict between containers, add cgroup ID or namespace to the lookup key
    bpf_map_update_elem(&root_nr_namespace_nr, &root_nr, &namespace_nr, BPF_ANY);
    bpf_map_update_elem(&namespace_nr_root_nr, &namespace_nr, &root_nr, BPF_ANY);
}

u32 __attribute__((always_inline)) get_root_nr(u32 namespace_nr) {
    // TODO(will): this can conflict between containers, add cgroup ID or namespace to the lookup key
    u32 *pid = bpf_map_lookup_elem(&namespace_nr_root_nr, &namespace_nr);
    return pid ? *pid : 0;
}

u32 __attribute__((always_inline)) get_namespace_nr(u32 root_nr) {
    // TODO(will): this can conflict between containers, add cgroup ID or namespace to the lookup key
    u32 *pid = bpf_map_lookup_elem(&root_nr_namespace_nr, &root_nr);
    return pid ? *pid : 0;
}

void __attribute__((always_inline)) remove_nr(u32 root_nr) {
    // TODO(will): this can conflict between containers, add cgroup ID or namespace to the lookup key
    u32 namespace_nr = get_namespace_nr(root_nr);
    if (root_nr == 0 || namespace_nr == 0) {
        return;
    }

    bpf_map_delete_elem(&root_nr_namespace_nr, &root_nr);
    bpf_map_delete_elem(&namespace_nr_root_nr, &namespace_nr);
}

u64 __attribute__((always_inline)) get_pid_level_offset() {
    u64 pid_level_offset;
    LOAD_CONSTANT("pid_level_offset", pid_level_offset);
    return pid_level_offset;
}

u64 __attribute__((always_inline)) get_pid_numbers_offset() {
    u64 pid_numbers_offset;
    LOAD_CONSTANT("pid_numbers_offset", pid_numbers_offset);
    return pid_numbers_offset;
}

u64 __attribute__((always_inline)) get_sizeof_upid() {
    u64 sizeof_upid;
    LOAD_CONSTANT("sizeof_upid", sizeof_upid);
    return sizeof_upid;
}

void __attribute__((always_inline)) cache_nr_translations(struct pid *pid) {
    if (pid == NULL) {
        return;
    }

    // read the root namespace nr from &pid->numbers[0].nr
    u32 root_nr = 0;
    bpf_probe_read(&root_nr, sizeof(root_nr), (void *)pid + get_pid_numbers_offset());

    // TODO(will): iterate over the list to insert the nr of each namespace, for now get only the deepest one
    u32 pid_level = 0;
    bpf_probe_read(&pid_level, sizeof(pid_level), (void *)pid + get_pid_level_offset());

    // read the namespace nr from &pid->numbers[pid_level].nr
    u32 namespace_nr = 0;
    u64 namespace_numbers_offset = pid_level * get_sizeof_upid();
    bpf_probe_read(&namespace_nr, sizeof(namespace_nr), (void *)pid + get_pid_numbers_offset() + namespace_numbers_offset);

    register_nr(root_nr, namespace_nr);
}

__attribute__((always_inline)) u32 get_ifindex_from_net_device(struct net_device *device) {
    u64 net_device_ifindex_offset;
    LOAD_CONSTANT("net_device_ifindex_offset", net_device_ifindex_offset);

    u32 ifindex;
    bpf_probe_read(&ifindex, sizeof(ifindex), (void*)device + net_device_ifindex_offset);
    return ifindex;
}

__attribute__((always_inline)) u32 get_netns_from_net(struct net *net) {
    u64 net_ns_offset;
    LOAD_CONSTANT("net_ns_offset", net_ns_offset);

    struct ns_common ns;
    bpf_probe_read(&ns, sizeof(ns), (void*)net + net_ns_offset);
    return ns.inum;
}

__attribute__((always_inline)) u32 get_netns_from_sock(struct sock *sk) {
    u64 sock_common_skc_net_offset;
    LOAD_CONSTANT("sock_common_skc_net_offset", sock_common_skc_net_offset);

    struct sock_common *common = (void *)sk;
    struct net *net = NULL;
    bpf_probe_read(&net, sizeof(net), (void *)common + sock_common_skc_net_offset);
    return get_netns_from_net(net);
}

__attribute__((always_inline)) u32 get_netns_from_socket(struct socket *socket) {
    u64 socket_sock_offset;
    LOAD_CONSTANT("socket_sock_offset", socket_sock_offset);

    struct sock *sk = NULL;
    bpf_probe_read(&sk, sizeof(sk), (void *)socket + socket_sock_offset);
    return get_netns_from_sock(sk);
}

struct namespace_switch_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
};

SEC("kprobe/switch_task_namespaces")
int kprobe_switch_task_namespaces(struct pt_regs *ctx) {
    struct nsproxy *new_ns = (struct nsproxy *)PT_REGS_PARM2(ctx);
    if (new_ns == NULL) {
        return 0;
    }

    struct net *net;
    bpf_probe_read(&net, sizeof(net), &new_ns->net_ns);
    if (net == NULL) {
        return 0;
    }

    u32 netns = get_netns_from_net(net);
    u32 tid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&netns_cache, &tid, &netns, BPF_ANY);

    struct namespace_switch_event_t evt = {};
    struct proc_cache_t *entry = fill_process_context(&evt.process);
    fill_container_context(entry, &evt.container);
    fill_span_context(&evt.span);

    send_event(ctx, EVENT_NAMESPACE_SWITCH, evt);
    return 0;
}

#endif
