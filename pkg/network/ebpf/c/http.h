#ifndef __HTTP_H
#define __HTTP_H

#include "tracer.h"
#include "http-types.h"
#include "http-maps.h"

#include <uapi/linux/ptrace.h>

static __always_inline void http_prepare_key(u32 cpu, http_batch_key_t *key, http_batch_state_t *batch_state) {
    __builtin_memset(key, 0, sizeof(http_batch_key_t));
    key->cpu = cpu;
    key->page_num = batch_state->idx % HTTP_BATCH_PAGES;
}

static __always_inline void http_notify_batch(struct pt_regs *ctx) {
    u32 cpu = bpf_get_smp_processor_id();

    http_batch_state_t *batch_state = bpf_map_lookup_elem(&http_batch_state, &cpu);
    if (batch_state == NULL || batch_state->idx_to_notify == batch_state->idx) {
        // batch is not ready to be flushed
        return;
    }

    // It's important to zero the struct so we account for the padding
    // introduced by the compilation, otherwise you get a `invalid indirect read
    // from stack off`. Alternatively we can either use a #pragma pack directive
    // or try to manually add the padding to the struct definition. More
    // information in https://docs.cilium.io/en/v1.8/bpf/ under the
    // alignment/padding section
    http_batch_notification_t notification = { 0 };
    notification.cpu = cpu;
    notification.batch_idx = batch_state->idx_to_notify;

    bpf_perf_event_output(ctx, &http_notifications, cpu, &notification, sizeof(http_batch_notification_t));
    log_debug("http batch notification flushed: cpu: %d idx: %d\n", notification.cpu, notification.batch_idx);
    batch_state->idx_to_notify++;
}

static __always_inline int http_responding(http_transaction_t *http) {
    return (http != NULL && http->response_status_code != 0);
}

static __always_inline void http_enqueue(http_transaction_t *http) {
    // Retrieve the active batch number for this CPU
    u32 cpu = bpf_get_smp_processor_id();
    http_batch_state_t *batch_state = bpf_map_lookup_elem(&http_batch_state, &cpu);
    if (batch_state == NULL) {
        return;
    }

    http_batch_key_t key;
    http_prepare_key(cpu, &key, batch_state);

    // Retrieve the batch object
    http_batch_t *batch = bpf_map_lookup_elem(&http_batches, &key);
    if (batch == NULL) {
        return;
    }

    // I haven't found a way to avoid this unrolled loop on Kernel 4.4 (newer versions work fine)
    // If you try to directly write the desired batch slot by doing
    //
    //  __builtin_memcpy(&batch->txs[batch_state->pos], http, sizeof(http_transaction_t));
    //
    // You get an error like the following:
    //
    // R0=inv R1=map_value(ks=4,vs=4816) R2=imm5 R3=imm0 R4=imm0 R6=map_value(ks=48,vs=96) R7=imm1 R8=imm0 R9=inv R10=fp
    ///809: (79) r2 = *(u64 *)(r6 +88)
    // 810: (7b) *(u64 *)(r0 +88) = r2
    // R0 invalid mem access 'inv'
    //
    // This is because the value range of the R0 register (holding the memory address of the batch) can't be
    // figured out by the verifier and thus the memory access can't be considered safe during verification time.
    // It seems that support for this type of access range by the verifier was added later on:
    // https://patchwork.ozlabs.org/project/netdev/patch/1475074472-23538-1-git-send-email-jbacik@fb.com/
    //
    // What is unfortunate about this is not only that enqueuing a HTTP transaction is O(HTTP_BATCH_SIZE),
    // but also that we can't really increase the batch/page size at the moment because that blows up the eBPF *program* size
#pragma unroll
    for (int i = 0; i < HTTP_BATCH_SIZE; i++) {
        if (i == batch_state->pos) {
            __builtin_memcpy(&batch->txs[i], http, sizeof(http_transaction_t));
        }
    }

    log_debug("http transaction enqueued: cpu: %d batch_idx: %d pos: %d\n", cpu, batch_state->idx, batch_state->pos);
    batch_state->pos++;

    // Copy batch state information for user-space
    batch->idx = batch_state->idx;
    batch->pos = batch_state->pos;

    // If we have filled the batch we move to the next one
    // Notice that we don't flush it directly because we can't do so from socket filter programs.
    if (batch_state->pos == HTTP_BATCH_SIZE) {
        batch_state->idx++;
        batch_state->pos = 0;
    }
}

static __always_inline void http_begin_request(http_transaction_t *http, http_method_t method, char *buffer) {
    http->request_method = method;
    http->request_started = bpf_ktime_get_ns();
    http->response_last_seen = 0;
    http->response_status_code = 0;
    __builtin_memcpy(&http->request_fragment, buffer, HTTP_BUFFER_SIZE);
}

static __always_inline void http_begin_response(http_transaction_t *http, const char *buffer) {
    u16 status_code = 0;
    status_code += (buffer[HTTP_STATUS_OFFSET+0]-'0') * 100;
    status_code += (buffer[HTTP_STATUS_OFFSET+1]-'0') * 10;
    status_code += (buffer[HTTP_STATUS_OFFSET+2]-'0') * 1;
    http->response_status_code = status_code;
}

static __always_inline void http_parse_data(char const *p, http_packet_t *packet_type, http_method_t *method) {
    if ((p[0] == 'H') && (p[1] == 'T') && (p[2] == 'T') && (p[3] == 'P')) {
        *packet_type = HTTP_RESPONSE;
    } else if ((p[0] == 'G') && (p[1] == 'E') && (p[2] == 'T') && (p[3]  == ' ') && (p[4] == '/')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_GET;
    } else if ((p[0] == 'P') && (p[1] == 'O') && (p[2] == 'S') && (p[3] == 'T') && (p[4]  == ' ') && (p[5] == '/')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_POST;
    } else if ((p[0] == 'P') && (p[1] == 'U') && (p[2] == 'T') && (p[3]  == ' ') && (p[4] == '/')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_PUT;
    } else if ((p[0] == 'D') && (p[1] == 'E') && (p[2] == 'L') && (p[3] == 'E') && (p[4] == 'T') && (p[5] == 'E') && (p[6]  == ' ') && (p[7] == '/')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_DELETE;
    } else if ((p[0] == 'H') && (p[1] == 'E') && (p[2] == 'A') && (p[3] == 'D') && (p[4]  == ' ') && (p[5] == '/')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_HEAD;
    } else if ((p[0] == 'O') && (p[1] == 'P') && (p[2] == 'T') && (p[3] == 'I') && (p[4] == 'O') && (p[5] == 'N') && (p[6] == 'S') && (p[7]  == ' ') && ((p[8] == '/') || (p[8] == '*'))) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_OPTIONS;
    } else if ((p[0] == 'P') && (p[1] == 'A') && (p[2] == 'T') && (p[3] == 'C') && (p[4] == 'H') && (p[5]  == ' ') && (p[6] == '/')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_PATCH;
    }
}


static __always_inline http_transaction_t *http_fetch_state(http_transaction_t *http, skb_info_t *skb_info, http_packet_t packet_type) {
    if (packet_type == HTTP_PACKET_UNKNOWN) {
        return bpf_map_lookup_elem(&http_in_flight, &http->tup);
    }

    // We detected either a request or a response
    // In this case we initialize (or fetch) state associated to this tuple
    bpf_map_update_elem(&http_in_flight, &http->tup, http, BPF_NOEXIST);
    http_transaction_t *http_ebpf = bpf_map_lookup_elem(&http_in_flight, &http->tup);
    if (http_ebpf == NULL || skb_info == NULL) {
        return http_ebpf;
    }

    // Bail out if we've seen this TCP segment before
    // This can happen in the context of localhost traffic where the same TCP segment
    // can be seen multiple times coming in and out from different interfaces
    if (http_ebpf->tcp_seq == skb_info->tcp_seq) {
        return NULL;
    }

    http_ebpf->tcp_seq = skb_info->tcp_seq;
    return http_ebpf;
}

static __always_inline http_transaction_t* http_should_flush_previous_state(http_transaction_t *http, http_packet_t packet_type) {
    // this can happen in the context of keep-alives
    bool must_flush = (packet_type == HTTP_REQUEST && http->request_started) ||
        (packet_type == HTTP_RESPONSE && http->response_status_code);

    if (!must_flush) {
        return NULL;
    }

    u32 cpu = bpf_get_smp_processor_id();
    http_batch_state_t *batch_state = bpf_map_lookup_elem(&http_batch_state, &cpu);
    if (batch_state == NULL) {
        return NULL;
    }

    __builtin_memcpy(&batch_state->scratch_tx, http, sizeof(http_transaction_t));
    return &batch_state->scratch_tx;
}

static __always_inline bool http_closed(http_transaction_t *http, skb_info_t *skb_info, u16 pre_norm_src_port) {
    return (skb_info && skb_info->tcp_flags&TCPHDR_FIN &&
            http->owned_by_src_port == pre_norm_src_port);
}

static __always_inline int http_process_fetched(http_transaction_t *http_stack,
                                               http_transaction_t *http_fetched,
                                               skb_info_t *skb_info,
                                               http_packet_t packet_type,
                                               http_method_t method,
                                               __u64 tags) {
    char *buffer = (char *)http_stack->request_fragment;
    if (http_fetched == NULL) {
        return 0;
    }

    http_transaction_t *to_flush = http_should_flush_previous_state(http_fetched, packet_type);
    if (packet_type == HTTP_REQUEST) {
        http_begin_request(http_fetched, method, buffer);
    } else if (packet_type == HTTP_RESPONSE) {
        http_begin_response(http_fetched, buffer);
    }

    http_fetched->tags |= tags;

    // If we have a (L7/application-layer) payload we want to update the response_last_seen
    // This is to prevent things such as a keep-alive adding up to the transaction latency
    if (buffer[0] != 0) {
        http_fetched->response_last_seen = bpf_ktime_get_ns();
    }

    bool conn_closed = http_closed(http_fetched, skb_info, http_stack->owned_by_src_port);
    if (conn_closed) {
        to_flush = http_fetched;
    }

    if (to_flush) {
        http_enqueue(to_flush);
    }

    if (conn_closed) {
        bpf_map_delete_elem(&http_in_flight, &http_stack->tup);
    }

    return 0;
}

static __always_inline int http_process(http_transaction_t *http_stack, skb_info_t *skb_info, __u64 tags) {
    char *buffer = (char *)http_stack->request_fragment;
    http_packet_t packet_type = HTTP_PACKET_UNKNOWN;
    http_method_t method = HTTP_METHOD_UNKNOWN;
    http_parse_data(buffer, &packet_type, &method);

    http_transaction_t *http_fetched = http_fetch_state(http_stack, skb_info, packet_type);
    return http_process_fetched(http_stack, http_fetched, skb_info, packet_type, method, tags);
}
#endif
