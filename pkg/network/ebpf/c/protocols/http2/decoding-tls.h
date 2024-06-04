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

    // Some functions might change and override data_off field in dispatcher_args_copy.skb_info. Since it is used as a key
    // in a map, we cannot allow it to be modified. Thus, storing the original value of the offset.
    __u32 original_off = pktbuf_data_offset(pkt);

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    pktbuf_map_lookup_option_t arr[] = {
        [PKTBUF_TLS] = {
            .map = &tls_http2_iterations,
            .key = &dispatcher_args_copy,
        },
    };
    http2_tail_call_state_t *tail_call_state = pktbuf_map_lookup(pkt, arr);
    if (tail_call_state == NULL) {
        // We didn't find the cached context, aborting.
        return 0;
    }

    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto delete_iteration;
    }

    http2_telemetry_t *http2_tel = get_telemetry(pkt);
    if (http2_tel == NULL) {
        goto delete_iteration;
    }

    http2_frame_with_offset *frames_array = tail_call_state->frames_array;
    http2_frame_with_offset current_frame;

    // create the http2 ctx for the current http2 frame.
    bpf_memset(http2_ctx, 0, sizeof(http2_ctx_t));
    http2_ctx->http2_stream_key.tup = dispatcher_args_copy.tup;
    normalize_tuple(&http2_ctx->http2_stream_key.tup);
    http2_ctx->dynamic_index.tup = dispatcher_args_copy.tup;

    http2_stream_t *current_stream = NULL;
    // Allocating an array of headers, to hold all interesting headers from the frame.
    http2_header_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        goto delete_iteration;
    }
    #pragma unroll(HTTP2_TLS_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL)
    for (__u16 index = 0; index < HTTP2_TLS_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL; index++) {
        if (tail_call_state->iteration >= tail_call_state->frames_count) {
            break;
        }
        // This check must be next to the access of the array, otherwise the verifier will complain.
        if (tail_call_state->iteration >= HTTP2_MAX_FRAMES_ITERATIONS) {
            break;
        }
        current_frame = frames_array[tail_call_state->iteration];
        tail_call_state->iteration += 1;

        if (current_frame.frame.type != kHeadersFrame) {
            continue;
        }

        http2_ctx->http2_stream_key.stream_id = current_frame.frame.stream_id;
        current_stream = http2_fetch_stream(&http2_ctx->http2_stream_key);
        if (current_stream == NULL) {
            continue;
        }
        pktbuf_set_offset(pkt, current_frame.offset);
        current_stream->tags |= args->tags;

        bpf_memset(headers_to_process, 0, HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING * sizeof(http2_header_t));
        __u8 interesting_headers = pktbuf_filter_relevant_headers(pkt, &dispatcher_args_copy.tup, &http2_ctx->dynamic_index, headers_to_process, current_frame.frame.length, http2_tel);
        pktbuf_process_headers(pkt, &http2_ctx->dynamic_index, current_stream, headers_to_process, interesting_headers, http2_tel);
    }

    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS &&
        tail_call_state->iteration < tail_call_state->frames_count &&
        tail_call_state->iteration < HTTP2_TLS_MAX_FRAMES_FOR_HEADERS_PARSER) {
        pktbuf_tail_call_option_t tail_call_arr[] = {
            [PKTBUF_SKB] = {
                .prog_array_map = &protocols_progs,
                .index = PROG_HTTP2_HEADERS_PARSER,
            },
            [PKTBUF_TLS] = {
                .prog_array_map = &tls_process_progs,
                .index = TLS_HTTP2_HEADERS_PARSER,
            },
        };
        pktbuf_tail_call_compact(pkt, tail_call_arr);
    }
    // Zeroing the iteration index to call EOS parser
    tail_call_state->iteration = 0;
    pktbuf_tail_call_option_t tail_call_arr[] = {
        [PKTBUF_SKB] = {
            .prog_array_map = &protocols_progs,
            .index = PROG_HTTP2_DYNAMIC_TABLE_CLEANER,
        },
        [PKTBUF_TLS] = {
            .prog_array_map = &tls_process_progs,
            .index = TLS_HTTP2_DYNAMIC_TABLE_CLEANER,
        },
    };
    pktbuf_tail_call_compact(pkt, tail_call_arr);

delete_iteration:
    // restoring the original value.
    pktbuf_set_offset(pkt, original_off);
    pktbuf_map_delete(pkt, arr);

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

    dynamic_counter_t *dynamic_counter = bpf_map_lookup_elem(&http2_dynamic_counter_table, &dispatcher_args_copy.tup);
    if (dynamic_counter == NULL) {
        goto next;
    }

    // We're checking if the difference between the current value of the dynamic global table, to the previous index we
    // cleaned, is bigger than our threshold. If so, we need to clean the table.
    if (dynamic_counter->value - dynamic_counter->previous <= HTTP2_DYNAMIC_TABLE_CLEANUP_THRESHOLD) {
        goto next;
    }

    dynamic_table_index_t dynamic_index = {
        .tup = dispatcher_args_copy.tup,
    };

    #pragma unroll(HTTP2_DYNAMIC_TABLE_CLEANUP_ITERATIONS)
    for (__u16 index = 0; index < HTTP2_DYNAMIC_TABLE_CLEANUP_ITERATIONS; index++) {
        // We should reserve the last HTTP2_DYNAMIC_TABLE_CLEANUP_THRESHOLD entries in the dynamic table.
        // So if we're about to delete an entry that is in the last HTTP2_DYNAMIC_TABLE_CLEANUP_THRESHOLD entries,
        // we should stop the cleanup.
        if (dynamic_counter->previous + HTTP2_DYNAMIC_TABLE_CLEANUP_THRESHOLD >= dynamic_counter->value) {
            break;
        }
        // Setting the current index.
        dynamic_index.index = dynamic_counter->previous;
        // Trying to delete the entry, it might not exist, so we're ignoring the return value.
        bpf_map_delete_elem(&http2_dynamic_table, &dynamic_index);
        // Incrementing the previous index.
        dynamic_counter->previous++;
    }

next:
    bpf_tail_call_compat(ctx, &tls_process_progs, TLS_HTTP2_EOS_PARSER);

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

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *tail_call_state = bpf_map_lookup_elem(&tls_http2_iterations, &dispatcher_args_copy);
    if (tail_call_state == NULL) {
        // We didn't find the cached context, aborting.
        return 0;
    }

    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&tls_http2_telemetry, &zero);
    if (http2_tel == NULL) {
        goto delete_iteration;
    }

    http2_frame_with_offset *frames_array = tail_call_state->frames_array;
    http2_frame_with_offset current_frame;

    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto delete_iteration;
    }
    bpf_memset(http2_ctx, 0, sizeof(http2_ctx_t));
    http2_ctx->http2_stream_key.tup = dispatcher_args_copy.tup;
    normalize_tuple(&http2_ctx->http2_stream_key.tup);

    bool is_rst = false, is_end_of_stream = false;
    http2_stream_t *current_stream = NULL;

    #pragma unroll(HTTP2_MAX_FRAMES_FOR_EOS_PARSER_PER_TAIL_CALL)
    for (__u16 index = 0; index < HTTP2_MAX_FRAMES_FOR_EOS_PARSER_PER_TAIL_CALL; index++) {
        if (tail_call_state->iteration >= HTTP2_MAX_FRAMES_ITERATIONS) {
            break;
        }

        current_frame = frames_array[tail_call_state->iteration];
        // Having this condition after assignment and not before is due to a verifier issue.
        if (tail_call_state->iteration >= tail_call_state->frames_count) {
            break;
        }
        tail_call_state->iteration += 1;

        is_rst = current_frame.frame.type == kRSTStreamFrame;
        is_end_of_stream = (current_frame.frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM;
        if (!is_rst && !is_end_of_stream) {
            continue;
        }

        http2_ctx->http2_stream_key.stream_id = current_frame.frame.stream_id;
        // A new stream must start with a request, so if it does not exist, we should not process it.
        current_stream = bpf_map_lookup_elem(&http2_in_flight, &http2_ctx->http2_stream_key);
        if (current_stream == NULL) {
            continue;
        }

        // When we accept an RST, it means that the current stream is terminated.
        // See: https://datatracker.ietf.org/doc/html/rfc7540#section-6.4
        // If rst, and stream is empty (no status code, or no response) then delete from inflight
        if (is_rst && (!current_stream->status_code.finalized || !current_stream->request_method.finalized || !current_stream->path.finalized)) {
            bpf_map_delete_elem(&http2_in_flight, &http2_ctx->http2_stream_key);
            continue;
        }

        if (is_rst) {
            __sync_fetch_and_add(&http2_tel->end_of_stream_rst, 1);
        } else if ((current_frame.frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) {
            __sync_fetch_and_add(&http2_tel->end_of_stream, 1);
        }
        handle_end_of_stream(current_stream, &http2_ctx->http2_stream_key, http2_tel);

        // If we reached here, it means that we saw End Of Stream. If the End of Stream came from a request,
        // thus we except it to have a valid path. If the End of Stream came from a response, we except it to
        // be after seeing a request, thus it should have a path as well.
        if ((!current_stream->path.finalized) || (!current_stream->request_method.finalized)) {
            bpf_map_delete_elem(&http2_in_flight, &http2_ctx->http2_stream_key);
        }
    }

    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS &&
        tail_call_state->iteration < tail_call_state->frames_count &&
        tail_call_state->iteration < HTTP2_MAX_FRAMES_FOR_EOS_PARSER) {
        bpf_tail_call_compat(ctx, &tls_process_progs, TLS_HTTP2_EOS_PARSER);
    }

delete_iteration:
    bpf_map_delete_elem(&tls_http2_iterations, &dispatcher_args_copy);

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
