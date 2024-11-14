#ifndef _HOOKS_NETWORK_NET_DEVICE_H_
#define _HOOKS_NETWORK_NET_DEVICE_H_

#include "constants/custom.h"
#include "constants/offsets/netns.h"
#include "helpers/process.h"
#include "helpers/span.h"
#include "maps.h"
#include "perf_ring.h"

int __attribute__((always_inline)) start_veth_state_machine() {
    u64 id = bpf_get_current_pid_tgid();
    struct veth_state_t state = {
        .state = STATE_NEWLINK,
    };
    bpf_map_update_elem(&veth_state_machine, &id, &state, BPF_ANY);
    return 0;
}

HOOK_ENTRY("rtnl_create_link")
int hook_rtnl_create_link(ctx_t *ctx) {
    struct rtnl_link_ops *ops = (struct rtnl_link_ops *)CTX_PARM4(ctx);
    if (!ops) {
        return 0;
    }

    char *kind_ptr;
    if (bpf_probe_read(&kind_ptr, sizeof(char *), &ops->kind) < 0 || !kind_ptr) {
        return 0;
    }

    char kind[5];
    if (bpf_probe_read_str(kind, 5, kind_ptr) < 0) {
        return 0;
    }

    if (kind[0] != 'v' || kind[1] != 'e' || kind[2] != 't' || kind[3] != 'h' || kind[4] != 0) {
        return 0;
    }

    return start_veth_state_machine();
}

HOOK_ENTRY("register_netdevice")
int hook_register_netdevice(ctx_t *ctx) {
    struct net_device *net_dev = (struct net_device *)CTX_PARM1(ctx);

    u64 id = bpf_get_current_pid_tgid();
    struct register_netdevice_cache_t entry = {
        .device = net_dev,
    };

    entry.ifindex.netns = get_netns_from_net_device(net_dev);

    bpf_map_update_elem(&register_netdevice_cache, &id, &entry, BPF_ANY);
    return 0;
};

HOOK_ENTRY("dev_get_valid_name")
int hook_dev_get_valid_name(ctx_t *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    struct register_netdevice_cache_t *entry = bpf_map_lookup_elem(&register_netdevice_cache, &id);
    if (entry != NULL) {
        struct net *net = (struct net *)CTX_PARM1(ctx);
        entry->ifindex.netns = get_netns_from_net(net);
    }
    return 0;
};

HOOK_ENTRY("dev_new_index")
int hook_dev_new_index(ctx_t *ctx) {
    u64 id = bpf_get_current_pid_tgid();

    struct register_netdevice_cache_t *entry = bpf_map_lookup_elem(&register_netdevice_cache, &id);
    if (entry != NULL) {
        struct net *net = (struct net *)CTX_PARM1(ctx);
        entry->ifindex.netns = get_netns_from_net(net);
    }
    return 0;
};

HOOK_EXIT("dev_new_index")
int rethook_dev_new_index(ctx_t *ctx) {
    u64 id = bpf_get_current_pid_tgid();

    struct register_netdevice_cache_t *entry = bpf_map_lookup_elem(&register_netdevice_cache, &id);
    if (entry != NULL) {
        entry->ifindex.ifindex = (u32)CTX_PARMRET(ctx, 1);
    }
    return 0;
};

HOOK_ENTRY("__dev_get_by_index")
int hook___dev_get_by_index(ctx_t *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    struct net *net = (struct net *)CTX_PARM1(ctx);

    struct device_ifindex_t entry = {
        .netns = get_netns_from_net(net),
        .ifindex = (u32)CTX_PARM2(ctx),
    };

    struct register_netdevice_cache_t *cache = bpf_map_lookup_elem(&register_netdevice_cache, &id);
    if (cache != NULL) {
        cache->ifindex = entry;
    }

    bpf_map_update_elem(&netdevice_lookup_cache, &id, &entry, BPF_ANY);
    return 0;
};

HOOK_EXIT("register_netdevice")
int rethook_register_netdevice(ctx_t *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    int ret = CTX_PARMRET(ctx, 1);
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
    char *name = get_net_device_name(entry->device);
    bpf_probe_read(&device.name, sizeof(device.name), name);

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
        struct device_ifindex_t lookup_key = state->peer_device_key; // for compatibility with older kernels
        struct device_t *peer_device = bpf_map_lookup_elem(&veth_devices, &lookup_key);
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

__attribute__((always_inline)) int trace_dev_change_net_namespace(ctx_t *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    struct net *net = (struct net *)CTX_PARM2(ctx);

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

HOOK_ENTRY("dev_change_net_namespace")
int hook_dev_change_net_namespace(ctx_t *ctx) {
    return trace_dev_change_net_namespace(ctx);
};

HOOK_ENTRY("__dev_change_net_namespace")
int hook___dev_change_net_namespace(ctx_t *ctx) {
    return trace_dev_change_net_namespace(ctx);
}

#endif
