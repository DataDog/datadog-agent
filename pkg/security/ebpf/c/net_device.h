#ifndef _NET_DEVICE_H_
#define _NET_DEVICE_H_

struct device_t {
    char name[16];
    u32 netns;
    u32 ifindex;
    u32 peer_netns;
    u32 peer_ifindex;
};

struct net_device_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct device_t device;
};

struct veth_pair_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct device_t host_device;
    struct device_t peer_device;
};

struct device_ifindex_t {
    u32 netns;
    u32 ifindex;
};

struct device_name_t {
    char name[16];
    u32 netns;
};

#define STATE_NULL 0
#define STATE_NEWLINK 1
#define STATE_REGISTER_PEER_DEVICE 2

struct veth_state_t {
    struct device_ifindex_t peer_device_key;
    u32 state;
};

struct bpf_map_def SEC("maps/veth_state_machine") veth_state_machine = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct veth_state_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/veth_devices") veth_devices = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct device_ifindex_t),
    .value_size = sizeof(struct device_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/veth_device_name_to_ifindex") veth_device_name_to_ifindex = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct device_name_t),
    .value_size = sizeof(struct device_ifindex_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct register_netdevice_cache_t {
    struct net_device *device;
    struct device_ifindex_t ifindex;
};

struct bpf_map_def SEC("maps/register_netdevice_cache") register_netdevice_cache = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct register_netdevice_cache_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/netdevice_lookup_cache") netdevice_lookup_cache = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct device_ifindex_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

SEC("kprobe/veth_newlink")
int kprobe_veth_newlink(struct pt_regs *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    struct veth_state_t state = {
        .state = STATE_NEWLINK,
    };
    bpf_map_update_elem(&veth_state_machine, &id, &state, BPF_ANY);
    return 0;
};

SEC("kprobe/register_netdevice")
int kprobe_register_netdevice(struct pt_regs *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    struct register_netdevice_cache_t entry = {
        .device = (struct net_device *)PT_REGS_PARM1(ctx),
    };
    bpf_map_update_elem(&register_netdevice_cache, &id, &entry, BPF_ANY);
    return 0;
};

SEC("kprobe/dev_get_valid_name")
int kprobe_dev_get_valid_name(struct pt_regs *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    struct register_netdevice_cache_t *entry = bpf_map_lookup_elem(&register_netdevice_cache, &id);
    if (entry != NULL) {
        struct net *net = (struct net *)PT_REGS_PARM1(ctx);
        entry->ifindex.netns = get_netns_from_net(net);
    }
    return 0;
};

SEC("kprobe/dev_new_index")
int kprobe_dev_new_index(struct pt_regs *ctx) {
    u64 id = bpf_get_current_pid_tgid();

    struct register_netdevice_cache_t *entry = bpf_map_lookup_elem(&register_netdevice_cache, &id);
    if (entry != NULL) {
        struct net *net = (struct net *)PT_REGS_PARM1(ctx);
        entry->ifindex.netns = get_netns_from_net(net);
    }
    return 0;
};

SEC("kretprobe/dev_new_index")
int kretprobe_dev_new_index(struct pt_regs *ctx) {
    u64 id = bpf_get_current_pid_tgid();

    struct register_netdevice_cache_t *entry = bpf_map_lookup_elem(&register_netdevice_cache, &id);
    if (entry != NULL) {
        entry->ifindex.ifindex = (u32)PT_REGS_RC(ctx);
    }
    return 0;
};

SEC("kprobe/__dev_get_by_index")
int kprobe___dev_get_by_index(struct pt_regs *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    struct net *net = (struct net *)PT_REGS_PARM1(ctx);

    struct device_ifindex_t entry = {
        .netns = get_netns_from_net(net),
        .ifindex = (u32)PT_REGS_PARM2(ctx),
    };

    struct register_netdevice_cache_t *cache = bpf_map_lookup_elem(&register_netdevice_cache, &id);
    if (cache != NULL) {
        cache->ifindex = entry;
    }

    bpf_map_update_elem(&netdevice_lookup_cache, &id, &entry, BPF_ANY);
    return 0;
};

SEC("kprobe/__dev_get_by_name")
int kprobe___dev_get_by_name(struct pt_regs *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    struct net *net = (struct net *)PT_REGS_PARM1(ctx);

    struct device_name_t name = {
        .netns = get_netns_from_net(net),
    };
    bpf_probe_read_str(&name.name[0], sizeof(name.name), (void *)PT_REGS_PARM2(ctx));

    struct device_ifindex_t *ifindex = bpf_map_lookup_elem(&veth_device_name_to_ifindex, &name);
    if (ifindex == NULL) {
        return 0;
    }

    struct device_ifindex_t entry = {
        .netns = name.netns,
        .ifindex = ifindex->ifindex,
    };

    bpf_map_update_elem(&netdevice_lookup_cache, &id, &entry, BPF_ANY);
    return 0;
};

SEC("kretprobe/register_netdevice")
int kretprobe_register_netdevice(struct pt_regs *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    int ret = PT_REGS_RC(ctx);
    if (ret != 0) {
        // interface registration failed, remove cache entry
        bpf_map_delete_elem(&register_netdevice_cache, &id);
        return 0;
    }

    // retrieve register_netdevice cache entry
    struct register_netdevice_cache_t *entry = bpf_map_lookup_elem(&register_netdevice_cache, &id);
    if (entry == NULL) {
        return 0;
    }

    // populate ifindex if need be
    if (entry->ifindex.ifindex == 0) {
        entry->ifindex.ifindex = get_ifindex_from_net_device(entry->device);
    }

    // prepare device key and device entry of newly registered device
    struct device_ifindex_t key = {
        .ifindex = entry->ifindex.ifindex,
        .netns = entry->ifindex.netns,
    };
    struct device_t device = {
        .ifindex = entry->ifindex.ifindex,
        .netns = entry->ifindex.netns,
    };
    // populate interface name directly from the net_device structure
    bpf_probe_read(&device.name[0], sizeof(device.name), entry->device);

    // check where we're at in the veth state machine
    struct veth_state_t *state = bpf_map_lookup_elem(&veth_state_machine, &id);
    if (state == NULL) {
        // this is a simple device registration
        struct net_device_event_t evt = {
            .device = device,
        };

        struct proc_cache_t *entry = fill_process_context(&evt.process);
        fill_container_context(entry, &evt.container);
        fill_span_context(&evt.span);

        send_event(ctx, EVENT_NET_DEVICE, evt);
        return 0;
    }

    // this is a veth pair, update the state machine
    switch (state->state) {
        case STATE_NEWLINK: {
            // this is the peer device
            state->peer_device_key = key;
            bpf_map_update_elem(&veth_devices, &key, &device, BPF_ANY);

            // update the veth state machine
            state->state = STATE_REGISTER_PEER_DEVICE;
            break;
        }

        case STATE_REGISTER_PEER_DEVICE: {
            // this is the host device
            struct device_t *peer_device = bpf_map_lookup_elem(&veth_devices, &state->peer_device_key);
            if (peer_device == NULL) {
                // peer device not found, should never happen
                return 0;
            }

            // update the peer device
            peer_device->peer_netns = key.netns;
            peer_device->peer_ifindex = key.ifindex;

            // insert new host device
            device.peer_netns = peer_device->netns;
            device.peer_ifindex = peer_device->ifindex;
            bpf_map_update_elem(&veth_devices, &key, &device, BPF_ANY);

            // delete state machine entry
            bpf_map_delete_elem(&veth_state_machine, &id);

            // veth pairs can be created with an existing peer netns, if this is the case, send the veth_pair event now
            if (peer_device->netns != device.netns) {
                // send event
                struct veth_pair_event_t evt = {
                    .host_device = device,
                    .peer_device = *peer_device,
                };

                struct proc_cache_t *proc_entry = fill_process_context(&evt.process);
                fill_container_context(proc_entry, &evt.container);
                fill_span_context(&evt.span);

                send_event(ctx, EVENT_VETH_PAIR, evt);
            }
            break;
        }
    }
    return 0;
};

__attribute__((always_inline)) void trace_dev_change_net_namespace(struct pt_regs *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    struct net *net = (struct net *)PT_REGS_PARM2(ctx);

    // lookup cache
    struct device_ifindex_t *ifindex = bpf_map_lookup_elem(&netdevice_lookup_cache, &id);
    if (ifindex == NULL) {
        return 0;
    }

    // lookup device
    struct device_ifindex_t key = *ifindex;
    struct device_t *device = bpf_map_lookup_elem(&veth_devices, &key);
    if (device == NULL) {
        return 0;
    }

    // lookup peer device
    key.netns = device->peer_netns;
    key.ifindex = device->peer_ifindex;
    struct device_t *peer_device = bpf_map_lookup_elem(&veth_devices, &key);
    if (peer_device == NULL) {
        return 0;
    }

    // update device with new netns
    device->netns = get_netns_from_net(net);
    peer_device->peer_netns = device->netns;

    // send event
    struct veth_pair_event_t evt = {
        .host_device = *peer_device,
        .peer_device = *device,
    };

    struct proc_cache_t *entry = fill_process_context(&evt.process);
    fill_container_context(entry, &evt.container);
    fill_span_context(&evt.span);

    send_event(ctx, EVENT_VETH_PAIR, evt);
    return 0;
}

SEC("kprobe/dev_change_net_namespace")
int kprobe_dev_change_net_namespace(struct pt_regs *ctx) {
    return trace_dev_change_net_namespace(ctx);
};

SEC("kprobe/__dev_change_net_namespace")
int kprobe___dev_change_net_namespace(struct pt_regs *ctx) {
    return trace_dev_change_net_namespace(ctx);
}

#endif
