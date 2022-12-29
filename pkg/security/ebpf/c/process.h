#ifndef _PROCESS_H_
#define _PROCESS_H_

#include <linux/tty.h>
#include <linux/sched.h>

#include "container.h"
#include "span.h"

struct process_entry_t {
    struct file_t executable;

    u64 exec_timestamp;
    char tty_name[TTY_NAME_LEN];
    char comm[TASK_COMM_LEN];
};

struct proc_cache_t {
    struct container_context_t container;
    struct process_entry_t entry;
};

static __attribute__((always_inline)) u32 copy_tty_name(const char src[TTY_NAME_LEN], char dst[TTY_NAME_LEN]) {
    if (src[0] == 0) {
        return 0;
    }

    bpf_probe_read(dst, TTY_NAME_LEN, (void*)src);
    return TTY_NAME_LEN;
}

void __attribute__((always_inline)) copy_proc_entry_except_comm(struct process_entry_t* src, struct process_entry_t* dst) {
    dst->executable = src->executable;
    dst->exec_timestamp = src->exec_timestamp;
    copy_tty_name(src->tty_name, dst->tty_name);
}

void __attribute__((always_inline)) copy_proc_cache(struct proc_cache_t *src, struct proc_cache_t *dst) {
    copy_container_id(src->container.container_id, dst->container.container_id);
    copy_proc_entry_except_comm(&src->entry, &dst->entry);
    bpf_probe_read(dst->entry.comm, TASK_COMM_LEN, src->entry.comm);
}

struct bpf_map_def SEC("maps/proc_cache") proc_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct proc_cache_t),
    .max_entries = 16384,
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
    .max_entries = 16384,
};

struct bpf_map_def SEC("maps/pid_ignored") pid_ignored = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 16738,
};

// defined in exec.h
struct proc_cache_t *get_proc_from_cookie(u32 cookie);

struct proc_cache_t * __attribute__((always_inline)) get_proc_cache(u32 tgid) {
    struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (!pid_entry) {
        return NULL;
    }

    // Select the cache entry
    return get_proc_from_cookie(pid_entry->cookie);
}

struct bpf_map_def SEC("maps/netns_cache") netns_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 40960,
};

static struct proc_cache_t * __attribute__((always_inline)) fill_process_context_with_pid_tgid(struct process_context_t *data, u64 pid_tgid) {
    u32 tgid = pid_tgid >> 32;

    // https://github.com/iovisor/bcc/blob/master/docs/reference_guide.md#4-bpf_get_current_pid_tgid
    data->pid = tgid;
    data->tid = pid_tgid;

    u32 tid = data->tid; // This looks unnecessary but it actually is to address this issue https://github.com/iovisor/bcc/issues/347 in at least Ubuntu 4.15.
    u32 *netns = bpf_map_lookup_elem(&netns_cache, &tid);
    if (netns != NULL) {
        data->netns = *netns;
    }

    u32 pid = data->pid;
    // consider kworker a pid which is ignored
    u32 *is_ignored = bpf_map_lookup_elem(&pid_ignored, &pid);
    if (is_ignored) {
        data->is_kworker = 1;
    }

    return get_proc_cache(tgid);
}

static struct proc_cache_t * __attribute__((always_inline)) fill_process_context(struct process_context_t *data) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    return fill_process_context_with_pid_tgid(data, pid_tgid);
}

struct bpf_map_def SEC("maps/root_nr_namespace_nr") root_nr_namespace_nr = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 32768,
};

struct bpf_map_def SEC("maps/namespace_nr_root_nr") namespace_nr_root_nr = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 32768,
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

#define NET_STRUCT_HAS_PROC_INUM 0
#define NET_STRUCT_HAS_NS        1

__attribute__((always_inline)) u32 get_netns_from_net(struct net *net) {
    u64 net_struct_type;
    LOAD_CONSTANT("net_struct_type", net_struct_type);
    u64 net_proc_inum_offset;
    LOAD_CONSTANT("net_proc_inum_offset", net_proc_inum_offset);
    u64 net_ns_offset;
    LOAD_CONSTANT("net_ns_offset", net_ns_offset);

    if (net_struct_type == NET_STRUCT_HAS_PROC_INUM) {
        u32 inum = 0;
        bpf_probe_read(&inum, sizeof(inum), (void*)net + net_proc_inum_offset);
        return inum;
    }

#ifndef DO_NOT_USE_TC
    struct ns_common ns;
    bpf_probe_read(&ns, sizeof(ns), (void*)net + net_ns_offset);
    return ns.inum;
#else
    return 0;
#endif
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

__attribute__((always_inline)) u32 get_netns_from_nf_conn(struct nf_conn *ct) {
    u64 nf_conn_ct_net_offset;
    LOAD_CONSTANT("nf_conn_ct_net_offset", nf_conn_ct_net_offset);

    struct net *net = NULL;
    bpf_probe_read(&net, sizeof(net), (void *)ct + nf_conn_ct_net_offset);
    return get_netns_from_net(net);
}

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
    return 0;
}

#endif
