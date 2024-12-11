#ifndef __HTTP2_DECODING_TLS_H
#define __HTTP2_DECODING_TLS_H

#include "protocols/http2/decoding-common.h"
#include "protocols/http2/usm-events.h"
#include "protocols/http/types.h"

// http2_tls_handle_first_frame is the entry point of our HTTP2+TLS processing.
// It is responsible for getting and filtering the first frame present in the
// buffer we get from the TLS uprobes.
//
// This first frame needs special handling as it may be split between multiple
// two buffers, and we may have the first part of the first frame from the
// processing of the previous buffer, in which case http2_tls_handle_first_frame
// will try to complete the frame.
//
// Once we have the first frame, we can continue to the regular frame filtering
// program.
SEC("uprobe/http2_tls_handle_first_frame")
int uprobe__http2_tls_handle_first_frame(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the
    // `off` field of tls_dispatcher_arguments, so the next prog will start to
    // read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_tls(ctx, &dispatcher_args_copy);

    handle_first_frame(pkt, (__u32*)&args->data_off, &dispatcher_args_copy.tup);
    return 0;
}

// http2_tls_filter finds and filters the HTTP2 frames from the buffer got from
// the TLS probes. Interesting frames are saved to be parsed in
// http2_tls_headers_parser.
SEC("uprobe/http2_tls_filter")
int uprobe__http2_tls_filter(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the
    // `off` field of the tls_dispatcher_arguments, so the next prog will start
    // to read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_tls(ctx, &dispatcher_args_copy);

    filter_frame(pkt, &dispatcher_args_copy, &dispatcher_args_copy.tup);
    return 0;
}


// The program is responsible for parsing all headers frames. For each headers frame we parse the headers,
// fill the dynamic table with the new interesting literal headers, and modifying the streams accordingly.
// The program can be called multiple times (via "self call" of tail calls) in case we have more frames to parse
// than the maximum number of frames we can process in a single tail call.
// The program is being called after uprobe__http2_tls_filter, and it is being called only if we have interesting frames.
// The program calls uprobe__http2_dynamic_table_cleaner to clean the dynamic table if needed.
SEC("uprobe/http2_tls_headers_parser")
int uprobe__http2_tls_headers_parser(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the
    // `off` field of tls_dispatcher_arguments, so the next prog will start to
    // read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_tls(ctx, &dispatcher_args_copy);

    headers_parser(pkt, &dispatcher_args_copy, &dispatcher_args_copy.tup, (__u8)args->tags);

    return 0;
}

// The program is responsible for cleaning the dynamic table.
// The program calls uprobe__http2_tls_eos_parser to finalize the streams and enqueue them to be sent to the user mode.
SEC("uprobe/http2_dynamic_table_cleaner")
int uprobe__http2_dynamic_table_cleaner(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the `off` field of skb_info, so
    // the next prog will start to read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_tls(ctx, &dispatcher_args_copy);
    dynamic_table_cleaner(pkt, &dispatcher_args_copy.tup);

    return 0;
}

// The program is responsible for parsing all frames that mark the end of a stream.
// We consider a frame as marking the end of a stream if it is either:
//  - An headers or data frame with END_STREAM flag set.
//  - An RST_STREAM frame.
// The program is being called after http2_dynamic_table_cleaner, and it finalizes the streams and enqueue them
// to be sent to the user mode.
// The program is ready to be called multiple times (via "self call" of tail calls) in case we have more frames to
// process than the maximum number of frames we can process in a single tail call.
SEC("uprobe/http2_tls_eos_parser")
int uprobe__http2_tls_eos_parser(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the `off` field of skb_info, so
    // the next prog will start to read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_tls(ctx, &dispatcher_args_copy);

    eos_parser(pkt, &dispatcher_args_copy, &dispatcher_args_copy.tup);

    return 0;
}

// http2_tls_termination is responsible for cleaning up the state of the HTTP2
// decoding once the TLS connection is terminated.
SEC("uprobe/http2_tls_termination")
int uprobe__http2_tls_termination(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    bpf_map_delete_elem(&tls_http2_iterations, &args->tup);

    terminated_http2_batch_enqueue(&args->tup);
    // Deleting the entry for the original tuple.
    bpf_map_delete_elem(&http2_remainder, &args->tup);
    bpf_map_delete_elem(&http2_dynamic_counter_table, &args->tup);
    // In case of local host, the protocol will be deleted for both (client->server) and (server->client),
    // so we won't reach for that path again in the code, so we're deleting the opposite side as well.
    flip_tuple(&args->tup);
    bpf_map_delete_elem(&http2_dynamic_counter_table, &args->tup);
    bpf_map_delete_elem(&http2_remainder, &args->tup);

    return 0;
}
#endif
