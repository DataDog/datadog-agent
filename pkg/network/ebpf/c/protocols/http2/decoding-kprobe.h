#ifndef __HTTP2_DECODING_KPROBE_H
#define __HTTP2_DECODING_KPROBE_H

#include "protocols/http2/decoding-common.h"
#include "protocols/http2/usm-events.h"
#include "protocols/http/types.h"

SEC("kprobe/http2_handle_first_frame")
int kprobe__http2_handle_first_frame(struct pt_regs *ctx) {
    const __u32 zero = 0;

    kprobe_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the
    // `off` field of tls_dispatcher_arguments, so the next prog will start to
    // read from the next valid frame.
    kprobe_dispatcher_arguments_t *args = bpf_map_lookup_elem(&kprobe_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_kprobe(ctx, &dispatcher_args_copy);

    handle_first_frame(pkt, (__u32*)&args->data_off, &dispatcher_args_copy.tup);
    return 0;
}

SEC("kprobe/http2_frame_filter")
int kprobe__http2_frame_filter(struct pt_regs *ctx) {
    const __u32 zero = 0;

    kprobe_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the
    // `off` field of the tls_dispatcher_arguments, so the next prog will start
    // to read from the next valid frame.
    kprobe_dispatcher_arguments_t *args = bpf_map_lookup_elem(&kprobe_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_kprobe(ctx, &dispatcher_args_copy);

    filter_frame(pkt, &dispatcher_args_copy, &dispatcher_args_copy.tup);
    return 0;
}

SEC("kprobe/http2_headers_parser")
int kprobe__http2_headers_parser(struct pt_regs *ctx) {
    const __u32 zero = 0;

    kprobe_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the
    // `off` field of tls_dispatcher_arguments, so the next prog will start to
    // read from the next valid frame.
    kprobe_dispatcher_arguments_t *args = bpf_map_lookup_elem(&kprobe_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_kprobe(ctx, &dispatcher_args_copy);

    headers_parser(pkt, &dispatcher_args_copy, &dispatcher_args_copy.tup, 0);

    return 0;
}

SEC("kprobe/http2_dynamic_table_cleaner")
int kprobe__http2_dynamic_table_cleaner(struct pt_regs *ctx) {
    const __u32 zero = 0;

    kprobe_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the `off` field of skb_info, so
    // the next prog will start to read from the next valid frame.
    kprobe_dispatcher_arguments_t *args = bpf_map_lookup_elem(&kprobe_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_kprobe(ctx, &dispatcher_args_copy);
    dynamic_table_cleaner(pkt, &dispatcher_args_copy.tup);

    return 0;
}

SEC("kprobe/http2_eos_parser")
int kprobe__http2_eos_parser(struct pt_regs *ctx) {
    const __u32 zero = 0;

    kprobe_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the `off` field of skb_info, so
    // the next prog will start to read from the next valid frame.
    kprobe_dispatcher_arguments_t *args = bpf_map_lookup_elem(&kprobe_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_kprobe(ctx, &dispatcher_args_copy);

    eos_parser(pkt, &dispatcher_args_copy, &dispatcher_args_copy.tup);

    return 0;
}

#endif
