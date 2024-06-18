#ifndef __PROTOCOL_CLASSIFICATION_SHARED_TRACER_MAPS_H
#define __PROTOCOL_CLASSIFICATION_SHARED_TRACER_MAPS_H

#include "map-defs.h"
#include "port_range.h"
#include "protocols/classification/stack-helpers.h"

// Maps a connection tuple to its classified protocol. Used to reduce redundant
// classification procedures on the same connection
BPF_HASH_MAP(connection_protocol, conn_tuple_t, protocol_stack_wrapper_t, 0)

static __always_inline protocol_stack_t* __get_protocol_stack(conn_tuple_t* tuple) {
    protocol_stack_wrapper_t *wrapper = bpf_map_lookup_elem(&connection_protocol, tuple);
    if (!wrapper) {
        return NULL;
    }
    return &wrapper->stack;
}

static __always_inline protocol_stack_t* get_protocol_stack(conn_tuple_t *skb_tup) {
    conn_tuple_t normalized_tup = *skb_tup;
    normalize_tuple(&normalized_tup);
    protocol_stack_wrapper_t* wrapper = bpf_map_lookup_elem(&connection_protocol, &normalized_tup);
    if (wrapper) {
        wrapper->updated = bpf_ktime_get_ns();
        return &wrapper->stack;
    }

    // this code path is executed once during the entire connection lifecycle
    protocol_stack_wrapper_t empty_wrapper = {0};
    empty_wrapper.updated = bpf_ktime_get_ns();

    // We skip EEXIST because of the use of BPF_NOEXIST flag. Emitting telemetry for EEXIST here spams metrics
    // and do not provide any useful signal since the key is expected to be present sometimes.
    //
    // EBUSY can be returned if a program tries to access an already held bucket lock
    // https://elixir.bootlin.com/linux/latest/source/kernel/bpf/hashtab.c#L164
    // Before kernel version 6.7 it was possible for a program to get interrupted before disabling
    // interrupts for acquring the bucket spinlock but after marking a bucket as busy.
    // https://github.com/torvalds/linux/commit/d35381aa73f7e1e8b25f3ed5283287a64d9ddff5
    // As such a program running from an irq context would falsely see a bucket as busy in certain cases
    // as explained in the linked commit message.
    //
    // Since connection_protocol is shared between maps running in different contexts, it gets effected by the
    // above scenario.
    // However the EBUSY error does not carry any signal for us since this is caused by a kernel bug.
    bpf_map_update_with_telemetry(&connection_protocol, &normalized_tup, &empty_wrapper, BPF_NOEXIST, -EEXIST, -EBUSY);
    return __get_protocol_stack(&normalized_tup);
}

__maybe_unused static __always_inline void update_protocol_stack(conn_tuple_t* skb_tup, protocol_t cur_fragment_protocol) {
    protocol_stack_t *stack = get_protocol_stack(skb_tup);
    if (!stack) {
        return;
    }

    set_protocol(stack, cur_fragment_protocol);
}

__maybe_unused static __always_inline void delete_protocol_stack(conn_tuple_t* normalized_tuple, protocol_stack_t *stack, u8 deletion_flag) {
    if (!normalized_tuple) {
        return;
    }

    if (!stack) {
        stack = bpf_map_lookup_elem(&connection_protocol, normalized_tuple);
        if (!stack) {
            return;
        }
    }

    if (!(stack->flags&FLAG_USM_ENABLED) || !(stack->flags&FLAG_NPM_ENABLED)) {
        // If either USM is disabled or NPM is disabled, we can move right away
        // to the deletion code since there is no chance of race between the two
        // programs.
        //
        // There are two expected scenarios where just one of the two programs
        // is enabled:
        //
        // 1) When one of the programs is disabled by choice (via configuration);
        //
        // 2) During system-probe startup: when system-probe is initializing
        // there is a short time window where the socket filter programs runs
        // alone *before* the `tcp_close` probe is activated. In a host with a
        // network-heavy workload this could easily result in thousands of
        // leaked entries.
        goto deletion;
    }

    // Otherwise we mark the protocol stack with the deletion flag
    //
    // In order to proceed with the deletion both the `tcp_close` probe and the
    // socket filter program must have reached this codepath, to ensure that
    // data is not prematurely deleted and both programs are able to handle the
    // termination path.
    //
    // Given that we're not using an atomic operation below, in the unlikely
    // event that tcp_close and the socket filter processing the FIN packet
    // execute at the same time, there is a chance that none of the callers
    // of this function will ever see both flags set.
    // We assume this is rare and OK since we're using an LRU map which will
    // eventually evict the leaked entry if it ever reaches capacity.
    //
    // Note that we could instead have a reference count field and increment it
    // attomically using the __sync_fetch_and_add builtin, which produces a
    // BPF_ATOMIC_ADD instruction. The problem is that this instruction requires
    // a 64-bit operand that would increase the size of of `protocol_stack_t` by
    // 3x. Since each `connection_tuple_t` embeds a `protocol_stack_t` that will
    // bloat the eBPF stack size for some of the tracer programs.
    //
    // In any case, even if we were using atomic operations, there is still a
    // chance of leak we can't avoid in the context of kprobe misses, so it's ok
    // to rely on the LRU in those cases.
    stack->flags |= deletion_flag;
    if (!(stack->flags&FLAG_TCP_CLOSE_DELETION) ||
        !(stack->flags&FLAG_SOCKET_FILTER_DELETION)) {
        return;
    }
 deletion:
    if (stack->flags&FLAG_SERVER_SIDE && stack->flags&FLAG_CLIENT_SIDE) {
        // If we reach this code path it means both client and server are
        // present in this host. To avoid a race condition where one side
        // potentially deletes protocol information before the other gets a
        // chance to retrieve it, we clear these flags and bail out, which
        // defers the deletion of protocol data to the last one to reach this
        // code path.
        stack->flags = stack->flags & ~(FLAG_SERVER_SIDE|FLAG_CLIENT_SIDE);
        return;
    }
    bpf_map_delete_elem(&connection_protocol, normalized_tuple);
}

#endif
