#ifndef __PROTOCOL_DISPATCHER_HELPERS_KPROBES_H
#define __PROTOCOL_DISPATCHER_HELPERS_KPROBES_H

#include "ktypes.h"

#include "ip.h"
#include "sock.h"

#include "protocols/classification/defs.h"
#include "protocols/classification/maps.h"
#include "protocols/classification/structs.h"
#include "protocols/classification/dispatcher-maps.h"
#include "protocols/http/classification-helpers.h"
#include "protocols/http/usm-events.h"
#include "protocols/http2/helpers.h"
#include "protocols/http2/usm-events.h"
#include "protocols/kafka/kafka-classification.h"
#include "protocols/kafka/usm-events.h"
#include "protocols/postgres/helpers.h"
#include "protocols/postgres/usm-events.h"
#include "protocols/redis/helpers.h"
#include "protocols/redis/usm-events.h"

static __always_inline void kprobe_protocol_dispatcher_entrypoint(struct pt_regs *ctx, struct sock *sock, const void *buffer, size_t bytes, bool receive) {
    conn_tuple_t tup = {0};

    u64 pid_tgid = bpf_get_current_pid_tgid();

    if (!read_conn_tuple(&tup, sock, pid_tgid, CONN_TYPE_TCP)) {
        log_debug("kprobe_protoco: could not read conn tuple");
        return;
    }

    if (receive) {
        __u64 tmp_h;
        __u64 tmp_l;

        // The tup data is read from the socker so source is always local but here
        // we are receveing data on the socket so flip things around.  Maybe this
        // could/should even come from the skb.
        tmp_h = tup.daddr_h;
        tmp_l = tup.daddr_l;
        tup.daddr_h = tup.saddr_h;
        tup.daddr_l = tup.saddr_l;
        tup.saddr_h = tmp_h;
        tup.saddr_l = tmp_l;

        __u16 tmp_port;
        tmp_port = tup.dport;
        tup.dport = tup.sport;
        tup.sport = tmp_port;
    }

    log_debug("kprobe tup: saddr: %08llx %08llx (%u)", tup.saddr_h, tup.saddr_l, tup.sport);
    log_debug("kprobe tup: daddr: %08llx %08llx (%u)", tup.daddr_h, tup.daddr_l, tup.dport);
    log_debug("kprobe tup: netns: %08x pid: %u", tup.netns, tup.pid);

    conn_tuple_t normalized_tuple = tup;
    normalize_tuple(&normalized_tuple);
    normalized_tuple.pid = 0;
    normalized_tuple.netns = 0;

    protocol_stack_t *stack = get_protocol_stack_if_exists(&normalized_tuple);

    protocol_t cur_fragment_protocol = get_protocol_from_stack(stack, LAYER_APPLICATION);
    if (is_protocol_layer_known(stack, LAYER_ENCRYPTION)) {
        // If we have a TLS connection, we can skip the packet.
        return;
    }

    if (cur_fragment_protocol == PROTOCOL_UNKNOWN) {
        log_debug("[kprobe_protocol_dispatcher_entrypoint]: %p was not classified", sock);
        char request_fragment[CLASSIFICATION_MAX_BUFFER];
        bpf_memset(request_fragment, 0, sizeof(request_fragment));
        // read_into_kernel_buffer_for_classification((char *)request_fragment, buffer);
        read_into_user_buffer_for_classification((char *)request_fragment, buffer);
        const size_t final_fragment_size = bytes < CLASSIFICATION_MAX_BUFFER ? bytes : CLASSIFICATION_MAX_BUFFER;
        classify_protocol_for_dispatcher(&cur_fragment_protocol, &tup, request_fragment, final_fragment_size);
        if (is_kafka_monitoring_enabled() && cur_fragment_protocol == PROTOCOL_UNKNOWN) {
            const __u32 zero = 0;
            kprobe_dispatcher_arguments_t *args = bpf_map_lookup_elem(&kprobe_dispatcher_arguments, &zero);
            if (args == NULL) {
                return;
            }
            *args = (kprobe_dispatcher_arguments_t){
                .tup = tup,
                .buffer_ptr = buffer,
                .data_end = bytes,
                .data_off = 0,
            };
            bpf_tail_call_compat(ctx, &kprobe_dispatcher_classification_progs, DISPATCHER_KAFKA_PROG);
        }
        log_debug("[kprobe_protocol_dispatcher_entrypoint]: %p Classifying protocol as: %d", sock, cur_fragment_protocol);
        // If there has been a change in the classification, save the new protocol.
        if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
            stack = get_or_create_protocol_stack(&normalized_tuple);
            if (!stack) {
                // should never happen, but it is required by the eBPF verifier
                return;
            }

            // This is used to signal the tracer program that this protocol stack
            // is also shared with our USM program for the purposes of deletion.
            // For more context refer to the comments in `delete_protocol_stack`
            set_protocol_flag(stack, FLAG_USM_ENABLED);
            set_protocol(stack, cur_fragment_protocol);
        }
    }

    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        conn_tuple_t *final_tuple = &tup;
        if (cur_fragment_protocol == PROTOCOL_HTTP) {
            final_tuple = &normalized_tuple;
        }

        const u32 zero = 0;
        kprobe_dispatcher_arguments_t *args = bpf_map_lookup_elem(&kprobe_dispatcher_arguments, &zero);
        if (args == NULL) {
            log_debug("dispatcher failed to save arguments for tail call");
            return;
        }

        bpf_memset(args, 0, sizeof(*args));
        bpf_memcpy(&args->tup, final_tuple, sizeof(conn_tuple_t));
        args->buffer_ptr = buffer;
        args->data_end = bytes;

        log_debug("kprobe_dispatching to protocol number: %d", cur_fragment_protocol);
        bpf_tail_call_compat(ctx, &kprobe_protocols_progs, protocol_to_program(cur_fragment_protocol));
    }
}

static __always_inline void kprobe_dispatch_kafka(struct pt_regs *ctx)
{
    log_debug("kprobe_dispatch_kafka");

    const __u32 zero = 0;
    kprobe_dispatcher_arguments_t *args = bpf_map_lookup_elem(&kprobe_dispatcher_arguments, &zero);
    if (args == NULL) {
        return;
    }

    char request_fragment[CLASSIFICATION_MAX_BUFFER];
    bpf_memset(request_fragment, 0, sizeof(request_fragment));

    conn_tuple_t normalized_tuple = args->tup;
    normalize_tuple(&normalized_tuple);
    normalized_tuple.pid = 0;
    normalized_tuple.netns = 0;

    read_into_user_buffer_for_classification(request_fragment, args->buffer_ptr);
    bool is_kafka = kprobe_is_kafka(ctx, args, request_fragment, CLASSIFICATION_MAX_BUFFER);
    log_debug("kprobe_dispatch_kafka: is_kafka %d", is_kafka);
    if (!is_kafka) {
        return;
    }

    update_protocol_stack(&normalized_tuple, PROTOCOL_KAFKA);
    bpf_tail_call_compat(ctx, &kprobe_protocols_progs, PROG_KAFKA);
}

#endif
