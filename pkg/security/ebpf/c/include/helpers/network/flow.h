#ifndef _HELPERS_NETWORK_FLOW_H
#define _HELPERS_NETWORK_FLOW_H

#include "maps.h"

__attribute__((always_inline)) struct sock_meta_t *reset_sock_meta(struct sock *sk) {
    struct sock_meta_t zero = {};
    if (is_sk_storage_supported()) {
        // This requires kernel v5.11+ (https://github.com/torvalds/linux/commit/8e4597c627fb48f361e2a5b012202cb1b6cbcd5e)
        struct sock_meta_t *meta = bpf_sk_storage_get(&sk_storage_meta, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
        if (meta != NULL) {
            *meta = zero;
        }
        return meta;
    } else {
        struct sock_meta_t *meta = bpf_map_lookup_elem(&sock_meta, &sk);
        if (meta == NULL) {

            #if defined(DEBUG_NETWORK_FLOW)
            bpf_printk("|    creating a new sock_meta for sock 0x%p", sk);
            #endif

            bpf_map_update_elem(&sock_meta, &sk, &zero, BPF_ANY);
        } else {
            *meta = zero;
        }
        return bpf_map_lookup_elem(&sock_meta, &sk);
    }
}

__attribute__((always_inline)) struct sock_meta_t *get_sock_meta(struct sock *sk) {
    if (is_sk_storage_supported()) {
        // This requires kernel v5.11+ (https://github.com/torvalds/linux/commit/8e4597c627fb48f361e2a5b012202cb1b6cbcd5e)
        return bpf_sk_storage_get(&sk_storage_meta, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    } else {
        struct sock_meta_t zero = {};
        struct sock_meta_t *meta = bpf_map_lookup_elem(&sock_meta, &sk);
        if (meta == NULL) {

            #if defined(DEBUG_NETWORK_FLOW)
            bpf_printk("|    creating a new sock_meta for sock 0x%p", sk);
            #endif

            bpf_map_update_elem(&sock_meta, &sk, &zero, BPF_ANY);
        }
        return bpf_map_lookup_elem(&sock_meta, &sk);
    }
}

__attribute__((always_inline)) struct sock_meta_t *peek_sock_meta(struct sock *sk) {
    if (is_sk_storage_supported()) {
        // This requires kernel v5.11+ (https://github.com/torvalds/linux/commit/8e4597c627fb48f361e2a5b012202cb1b6cbcd5e)
        return bpf_sk_storage_get(&sk_storage_meta, sk, 0, 0);
    } else {
        return bpf_map_lookup_elem(&sock_meta, &sk);
    }
}

__attribute__((always_inline)) void delete_sock_meta(struct sock *sk) {
    if (!is_sk_storage_supported()) {
        bpf_map_delete_elem(&sock_meta, &sk);
    }
}

__attribute__((always_inline)) void print_meta(struct sock_meta_t *meta) {
#if defined(DEBUG_NETWORK_FLOW)
    bpf_printk("|    sock_meta:");
    bpf_printk("|        route: p:%d a:%lu a:%lu", meta->existing_route.port, meta->existing_route.addr[0], meta->existing_route.addr[1]);
#endif
}

__attribute__((always_inline)) void print_route_entry(struct pid_route_entry_t *route_entry) {
#if defined(DEBUG_NETWORK_FLOW)
    bpf_printk("|    route_entry:");
    bpf_printk("|        pid:%d type:%d owner_sk:0x%p", route_entry->pid, route_entry->type, route_entry->owner_sk);
#endif
}

__attribute__((always_inline)) void print_route(struct pid_route_t *route) {
#if defined(DEBUG_NETWORK_FLOW)
    bpf_printk("|    route:");
    bpf_printk("|        p:%d a:%lu a:%lu", htons(route->port), route->addr[0], route->addr[1]);
    bpf_printk("|        netns:%lu", route->netns);
    bpf_printk("|        protocol:%lu", route->l4_protocol);
#endif
}

__attribute__((always_inline)) u8 can_delete_route(struct pid_route_t *route, struct sock *sk) {
    struct pid_route_entry_t *existing_entry = bpf_map_lookup_elem(&flow_pid, route);
    if (existing_entry != NULL) {
        #if defined(DEBUG_NETWORK_FLOW)
        bpf_printk("|    - attempting to delete:");
        print_route_entry(existing_entry);
        #endif

        if (existing_entry->type == PROCFS_ENTRY) {
            // we have no restriction for deleting proc fd entries
            return 1;
        }
        if (existing_entry->owner_sk == sk) {
            return 1;
        }
    } else {
        #if defined(DEBUG_NETWORK_FLOW)
        bpf_printk("|    - no entry found for input route:");
        print_route(route);
        #endif

        return 1;
    }
    return 0;
}

#endif
