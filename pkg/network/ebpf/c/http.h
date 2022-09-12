#ifndef __HTTP_H
#define __HTTP_H

#include "tracer.h"
#include "http-types.h"
#include "http-maps.h"
#include "https.h"

#include <uapi/linux/ptrace.h>

static __always_inline void http_prepare_key(u32 cpu, http_batch_key_t *key, http_batch_state_t *batch_state) {
    __builtin_memset(key, 0, sizeof(http_batch_key_t));
    key->cpu = cpu;
    key->page_num = batch_state->idx % HTTP_BATCH_PAGES;
}

static __always_inline void http_flush_batch(struct pt_regs *ctx) {
    u32 zero = 0;
    http_batch_state_t *batch_state = bpf_map_lookup_elem(&http_batch_state, &zero);
    if (batch_state == NULL || batch_state->idx_to_flush == batch_state->idx) {
        // batch is not ready to be flushed
        return;
    }

    u32 cpu = bpf_get_smp_processor_id();
    http_batch_key_t key = {0};
    key.cpu = cpu;
    key.page_num = batch_state->idx_to_flush % HTTP_BATCH_PAGES;
    http_batch_t *batch = bpf_map_lookup_elem(&http_batches, &key);
    if (batch == NULL) {
        return;
    }

    bpf_perf_event_output(ctx, &http_batch_events, cpu, batch, sizeof(http_batch_t));
    log_debug("http batch flushed: cpu: %d idx: %d\n", cpu, batch->idx);
    batch_state->idx_to_flush++;
}

static __always_inline int http_responding(http_transaction_t *http) {
    return (http != NULL && http->response_status_code != 0);
}

static __always_inline void http_enqueue(http_transaction_t *http) {
    // Retrieve the active batch number for this CPU
    u32 zero = 0;
    http_batch_state_t *batch_state = bpf_map_lookup_elem(&http_batch_state, &zero);
    if (batch_state == NULL) {
        return;
    }

    u32 cpu = bpf_get_smp_processor_id();
    http_batch_key_t key;
    http_prepare_key(cpu, &key, batch_state);

    // Retrieve the batch object
    http_batch_t *batch = bpf_map_lookup_elem(&http_batches, &key);
    if (batch == NULL) {
        return;
    }

    if (!(batch_state->pos >= 0 && batch_state->pos < HTTP_BATCH_SIZE)) {
        return;
    }

    __builtin_memcpy(&batch->txs[batch_state->pos], http, sizeof(http_transaction_t));
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

static __always_inline bool http_should_flush_previous_state(http_transaction_t *http, http_packet_t packet_type) {
    return (packet_type == HTTP_REQUEST && http->request_started) ||
        (packet_type == HTTP_RESPONSE && http->response_status_code);
}

static __always_inline bool http_closed(http_transaction_t *http, skb_info_t *skb_info, u16 pre_norm_src_port) {
    return (skb_info && skb_info->tcp_flags&(TCPHDR_FIN|TCPHDR_RST) &&
            // This is done to avoid double flushing the same
            // `http_transaction_t` to userspace.  In the context of a regular
            // TCP teardown, the FIN flag will be seen in "both ways", like:
            //
            // server -> FIN -> client
            // server <- FIN <- client
            //
            // Since we can't make any assumptions about the ordering of these
            // events and there are no synchronization primitives available to
            // us, the way we solve it is by storing the non-normalized src port
            // when we start tracking a HTTP transaction and ensuring that only the
            // FIN flag seen in the same direction will trigger the flushing event.
            http->owned_by_src_port == pre_norm_src_port);
}

static __always_inline int http_process(http_transaction_t *http_stack, skb_info_t *skb_info, __u64 tags) {
    char *buffer = (char *)http_stack->request_fragment;
    http_packet_t packet_type = HTTP_PACKET_UNKNOWN;
    http_method_t method = HTTP_METHOD_UNKNOWN;
    http_parse_data(buffer, &packet_type, &method);

    http_transaction_t *http = http_fetch_state(http_stack, skb_info, packet_type);
    if (http == NULL) {
        return 0;
    }

    if (http_should_flush_previous_state(http, packet_type)) {
        http_enqueue(http);
    }

    if (packet_type == HTTP_REQUEST) {
        http_begin_request(http, method, buffer);
    } else if (packet_type == HTTP_RESPONSE) {
        http_begin_response(http, buffer);
    }

    http->tags |= tags;

    if (http_responding(http)) {
        http->response_last_seen = bpf_ktime_get_ns();
    }

    if (http_closed(http, skb_info, http_stack->owned_by_src_port)) {
        http_enqueue(http);
        bpf_map_delete_elem(&http_in_flight, &http_stack->tup);
    }

    return 0;
}

// this function is called by the socket-filter program to decide whether or not we should inspect
// the contents of a certain packet, in order to avoid the cost of processing packets that are not
// of interest such as empty ACKs, UDP data or encrypted traffic.
static __always_inline bool http_allow_packet(http_transaction_t *http, struct __sk_buff* skb, skb_info_t *skb_info) {
    // we're only interested in TCP traffic
    if (!(http->tup.metadata&CONN_TYPE_TCP)) {
        return false;
    }

    // if payload data is empty or if this is an encrypted packet, we only
    // process it if the packet represents a TCP termination
    bool empty_payload = skb_info->data_off == skb->len;
    if (empty_payload || http->tup.sport == HTTPS_PORT || http->tup.dport == HTTPS_PORT) {
        return skb_info->tcp_flags&(TCPHDR_FIN|TCPHDR_RST);
    }

    return true;
}


#endif
