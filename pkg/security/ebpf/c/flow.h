#ifndef _FLOW_H_
#define _FLOW_H_

struct flow_pid_key_t {
    u64 addr[2];
    u16 port;
};

struct flow_pid_value_t {
    u32 pid;
};

struct bpf_map_def SEC("maps/flow_pid") flow_pid = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct flow_pid_key_t),
    .value_size = sizeof(struct flow_pid_value_t),
    .max_entries = 10240,
    .pinning = 0,
    .namespace = "",
};

#define EGRESS 1
#define INGRESS 2

__attribute__((always_inline)) u32 get_flow_pid(struct flow_pid_key_t *key) {
    struct flow_pid_value_t *value = bpf_map_lookup_elem(&flow_pid, key);
    if (!value) {
        // Try with IP set to 0.0.0.0
        key->addr[0] = 0;
        key->addr[1] = 0;
        value = bpf_map_lookup_elem(&flow_pid, key);
        if (!value) {
            return 0;
        }
    }

    return value->pid;
}

SEC("kprobe/security_sk_classify_flow")
int kprobe_security_sk_classify_flow(struct pt_regs *ctx)
{
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    struct flowi *fl = (struct flowi *)PT_REGS_PARM2(ctx);
    struct flow_pid_key_t key = {};
    u16 family = 0;

    bpf_probe_read(&family, sizeof(family), &sk->sk_family);
    union flowi_uli uli;
    if (family == AF_INET6) {
        struct flowi6 ip6;
        bpf_probe_read(&ip6, sizeof(ip6), &fl->u.ip6);
        bpf_probe_read(&uli, sizeof(uli), &ip6.uli);
        bpf_probe_read(&key.port, sizeof(key.port), &uli.ports.sport);
        bpf_probe_read(&key.addr, sizeof(u64) * 2, &ip6.saddr);
    } else if (family == AF_INET) {
        struct flowi4 ip4;
        bpf_probe_read(&ip4, sizeof(ip4), &fl->u.ip4);
        bpf_probe_read(&uli, sizeof(uli), &ip4.uli);
        bpf_probe_read(&key.port, sizeof(key.port), &uli.ports.sport);
        bpf_probe_read(&key.addr, sizeof(sk->__sk_common.skc_rcv_saddr), &sk->__sk_common.skc_rcv_saddr);
    } else {
        return 0;
    }

    // Register service PID
    if (key.port != 0) {
        struct flow_pid_value_t value = {
            .pid = bpf_get_current_pid_tgid() >> 32,
        };
        bpf_map_update_elem(&flow_pid, &key, &value, BPF_ANY);

#ifdef DEBUG
        bpf_printk("# registered (flow) pid:%d\n", value.pid);
        bpf_printk("# p:%d a:%d a:%d\n", key.port, key.addr[0], key.addr[1]);
#endif
    }
    return 0;
};

SEC("kprobe/security_socket_bind")
int kprobe_security_socket_bind(struct pt_regs *ctx)
{
    struct sockaddr *address = (struct sockaddr *)PT_REGS_PARM2(ctx);
    struct flow_pid_key_t key = {};
    u16 family = 0;

    // Extract IP and port from the sockaddr structure
    bpf_probe_read(&family, sizeof(family), &address->sa_family);
    if (family == AF_INET) {
        struct sockaddr_in *addr_in = (struct sockaddr_in *)address;
        bpf_probe_read(&key.port, sizeof(addr_in->sin_port), &addr_in->sin_port);
        bpf_probe_read(&key.addr, sizeof(addr_in->sin_addr.s_addr), &addr_in->sin_addr.s_addr);
    } else if (family == AF_INET6) {
        struct sockaddr_in6 *addr_in6 = (struct sockaddr_in6 *)address;
        bpf_probe_read(&key.port, sizeof(addr_in6->sin6_port), &addr_in6->sin6_port);
        bpf_probe_read(&key.addr, sizeof(u64) * 2, (char *)addr_in6 + offsetof(struct sockaddr_in6, sin6_addr));
    } else {
        return 0;
    }

    // Register service PID
    if (key.port != 0) {
        struct flow_pid_value_t value = {
            .pid = bpf_get_current_pid_tgid() >> 32,
        };
        bpf_map_update_elem(&flow_pid, &key, &value, BPF_ANY);

#ifdef DEBUG
        bpf_printk("# registered (bind) pid:%d\n", value.pid);
        bpf_printk("# p:%d a:%d a:%d\n", key.port, key.addr[0], key.addr[1]);
#endif
    }
    return 0;
};

#endif
