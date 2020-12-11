#ifndef __HTTP_H
#define __HTTP_H

#include "tracer.h"
#include "bpf_helpers.h"
#include "tracer-maps.h"

static __always_inline void http_prepare_key(u32 cpu, http_batch_key_t *key, http_batch_state_t *batch_state) {
    __builtin_memset(key, 0, sizeof(http_batch_key_t));
    key->cpu = cpu;
    key->page_num = batch_state->idx % HTTP_BATCH_PAGES;
}

static __always_inline void http_notify_batch(struct pt_regs* ctx) {
    u32 cpu = bpf_get_smp_processor_id();

    http_batch_state_t *batch_state = bpf_map_lookup_elem(&http_batch_state, &cpu);
    if (batch_state == NULL || batch_state->pos < HTTP_BATCH_SIZE) {
        return;
    }

    http_batch_key_t key;
    http_prepare_key(cpu, &key, batch_state);

    http_batch_t *batch = bpf_map_lookup_elem(&http_batches, &key);
    if (batch == NULL) {
        return;
    }

    // It's important to zero the struct so we account for the padding
    // introduced by the compilation, otherwise you get a `invalid indirect read
    // from stack off`. Alternatively we can either use a #pragma pack directive
    // or try to manually add the padding to the struct definition. More
    // information in https://docs.cilium.io/en/v1.8/bpf/ under the
    // alignment/padding section
    http_batch_notification_t notification = {0};
    notification.cpu = cpu;
    notification.batch_idx = batch_state->idx;

    bpf_perf_event_output(ctx, &http_notifications, cpu, &notification, sizeof(http_batch_notification_t));
    log_debug("http batch notification flushed: cpu: %d idx: %d lost_events: %d\n", cpu, batch->idx, batch_state->pos-HTTP_BATCH_SIZE);
    batch_state->idx++;
    batch_state->pos = 0;
}

static __always_inline int http_responding(http_transaction_t *http) {
    return (http != NULL && http->response_status_code != 0);
}

static __always_inline void http_end_response(http_transaction_t *http) {
    if (!http_responding(http)) {
        return;
    }

    // Retrieve the active batch number for this CPU
    u32 cpu = bpf_get_smp_processor_id();
    http_batch_state_t *batch_state = bpf_map_lookup_elem(&http_batch_state, &cpu);
    if (batch_state == NULL) {
        log_debug("http batch state not found. should not happen.");
        return;
    }

    if (batch_state->pos >= HTTP_BATCH_SIZE) {
        // We keep incrementing this so we can track how many transactions we're dropping
        batch_state->pos++;
        return;
    }

    http_batch_key_t key;
    http_prepare_key(cpu, &key, batch_state);

    // Retrieve the batch object
    http_batch_t *batch = bpf_map_lookup_elem(&http_batches, &key);
    if (batch == NULL) {
        log_debug("http batch not found. should not happen. cpu: %d page: %d\n", key.cpu, key.page_num);
        return;
    }

    // This redundant information is useful for detecting dirty batch pages on userspace without
    // incurring on an extra map lookup
    batch->idx = batch_state->idx;

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
    // I tried different "tricks" to hint the verifier of this range such as:
    //
    // * Ensuring 0 <= batch_slot < HTTP_BATCH_SIZE
    // * Ensuring that &batch <= &batch->txs[batch_state->pos] <= &batch+1
    // * Setting HTTP_BATCH_SIZEto a power of 2 and doing &batch->txs[batch_slot&(HTTP_BATCH_PAGES-1)]
    //
    // Among other things, but nothing really worked on Kernel 4.4
    // It seems that indeed support for this type of access by the verifier was added later on:
    // https://patchwork.ozlabs.org/project/netdev/patch/1475074472-23538-1-git-send-email-jbacik@fb.com/
    //
    // What is unfortunate about this is not only that enqueing a HTTP transaction is O(HTTP_BATCH_SIZE),
    // but also that we can't really increase the batch/page size at the moment because that blows up the eBPF *program* size
#pragma unroll
    for (int i = 0; i < HTTP_BATCH_SIZE; i++) {
        if (i == batch_state->pos) {
            __builtin_memcpy(&batch->txs[i], http, sizeof(http_transaction_t));
        }
    }

    batch_state->pos++;
    log_debug("http response ended: code: %d duration: %d(ms)\n", http->response_status_code, (http->response_last_seen-http->request_started)/(1000*1000));
}

static __always_inline int http_begin_request(http_transaction_t *http, http_method_t method, char *buffer) {
    // This can happen in the context of HTTP keep-alives;
    if (http_responding(http)) {
        http_end_response(http);
    }

    log_debug("http request started\n");
    http->request_method = method;
    http->request_started = bpf_ktime_get_ns();
    http->response_last_seen = 0;
    http->response_status_code = 0;
    __builtin_memcpy(&http->request_fragment, buffer, HTTP_BUFFER_SIZE);
    return 1;
}

static __always_inline int http_begin_response(http_transaction_t *http, char *buffer) {
    // We missed the corresponding request so nothing to do
    if (!(http->request_started)) {
        return 0;
    }

    // Extract the status code from the response fragment
    // HTTP/1.1 200 OK
    // _________^^^___
    // Code below is a bit oddly structured in order to make kernel 4.4 verifier happy
    __u16 status_code = 0;
    __u8 space_found = 0;
#pragma unroll
    for (int i = 0; i < HTTP_BUFFER_SIZE-1; i++) {
        if (!space_found && buffer[i] == ' ') {
            space_found = 1;
        } else if (space_found && status_code < 100) {
            status_code = status_code*10 + (buffer[i]-'0');
        }
    }

    if (status_code < 100 || status_code >= 600) {
        return 0;
    }

    http->response_status_code = status_code;
    log_debug("http response started: code: %d\n", http->response_status_code);
    return 1;
}

static __always_inline void http_read_data(struct __sk_buff* skb, skb_info_t* skb_info, char* p, http_packet_t* packet_type, http_method_t* method) {
#pragma unroll
    for (int i = 0; i < HTTP_BUFFER_SIZE; i++) {
        if (skb_info->data_off + i <= skb->len-1) {
            p[i] = load_byte(skb, skb_info->data_off + i);
        } else {
            // TODO: follow-up on why the verifier rejects the program without this else branch
            // even if you memset the buffer before
            p[i] = '\0';
        }
    }

    if ((p[0] == 'H') && (p[1] == 'T') && (p[2] == 'T') && (p[3] == 'P')) {
        *packet_type = HTTP_RESPONSE;
    } else if ((p[0] == 'G') && (p[1] == 'E') && (p[2] == 'T')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_GET;
    } else if ((p[0] == 'P') && (p[1] == 'O') && (p[2] == 'S') && (p[3] == 'T')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_POST;
    } else if ((p[0] == 'P') && (p[1] == 'U') && (p[2] == 'T')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_POST;
    } else if ((p[0] == 'D') && (p[1] == 'E') && (p[2] == 'L') && (p[3] == 'E') && (p[4] == 'T') && (p[5] == 'E')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_DELETE;
    } else if ((p[0] == 'H') && (p[1] == 'E') && (p[2] == 'A') && (p[3] == 'D')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_HEAD;
    } else if ((p[0] == 'O') && (p[1] == 'P') && (p[2] == 'T') && (p[3] == 'I') && (p[4] == 'O') && (p[5] == 'N') && (p[6] == 'S')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_OPTIONS;
    } else if ((p[0] == 'P') && (p[1] == 'A') && (p[2] == 'T') && (p[3] == 'C') && (p[4] == 'H')) {
        *packet_type = HTTP_REQUEST;
        *method = HTTP_PATCH;
    }
}

static __always_inline int http_handle_packet(struct __sk_buff* skb, skb_info_t* skb_info) {
    char buffer[HTTP_BUFFER_SIZE];
    __builtin_memset(&buffer, '\0', sizeof(HTTP_BUFFER_SIZE));

    http_packet_t packet_type = HTTP_PACKET_UNKNOWN;
    http_method_t method = HTTP_METHOD_UNKNOWN;
    http_read_data(skb, skb_info, buffer, &packet_type, &method);

    if (packet_type == HTTP_REQUEST) {
        // Ensure the creation of a http_transaction_t entry for tracking this request
        http_transaction_t new_entry = {};
        __builtin_memcpy(&new_entry.tup, &skb_info->tup, sizeof(conn_tuple_t));
        bpf_map_update_elem(&http_in_flight, &skb_info->tup, &new_entry, BPF_NOEXIST);
    }

    http_transaction_t *http = bpf_map_lookup_elem(&http_in_flight, &skb_info->tup);
    if (http == NULL) {
        // This happens when we lose the beginning of a HTTP request
        return 0;
    }

    if (packet_type == HTTP_REQUEST) {
        // We intercepted the first segment of the HTTP *request*
        http_begin_request(http, method, buffer);
    } else if (packet_type == HTTP_RESPONSE) {
        // We intercepted the first segment of the HTTP *response*
        http_begin_response(http, buffer);
    }

    if (http_responding(http)) {
        if (skb->len-1 > skb_info->data_off) {
            // Only if we have a (L7/application-layer) payload we want to update the response_last_seen
            // This is to prevent things such as a keep-alive adding up to the transaction latency
            http->response_last_seen = bpf_ktime_get_ns();
        }

        if (skb_info->tcp_flags&TCPHDR_FIN) {
            // The HTTP response has ended
            http_end_response(http);
            bpf_map_delete_elem(&http_in_flight, &skb_info->tup);
        }
    }

    return 0;
}

#endif
