#ifndef __HTTP_H
#define __HTTP_H

#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#include "protocols/sockfd.h"

#include "protocols/classification/common.h"

#include "protocols/http/types.h"
#include "protocols/http/maps.h"
#include "protocols/http/usm-events.h"
#include "protocols/tls/https.h"

static __always_inline int http_responding(http_transaction_t *http) {
    return (http != NULL && http->response_status_code != 0);
}

static __always_inline void http_begin_request(http_transaction_t *http, http_method_t method, char *buffer) {
    http->request_method = method;
    http->request_started = bpf_ktime_get_ns();
    http->response_last_seen = 0;
    http->response_status_code = 0;
    bpf_memcpy(&http->request_fragment, buffer, HTTP_BUFFER_SIZE);
    log_debug("http_begin_request: htx=%llx method=%d start=%llx", http, http->request_method, http->request_started);
}

static __always_inline void http_begin_response(http_transaction_t *http, const char *buffer) {
    u16 status_code = 0;
    status_code += (buffer[HTTP_STATUS_OFFSET+0]-'0') * 100;
    status_code += (buffer[HTTP_STATUS_OFFSET+1]-'0') * 10;
    status_code += (buffer[HTTP_STATUS_OFFSET+2]-'0') * 1;
    http->response_status_code = status_code;
    log_debug("http_begin_response: htx=%llx status=%d", http, status_code);
}

static __always_inline void http_batch_enqueue_wrapper(conn_tuple_t *tuple, http_transaction_t *http) {
    u32 zero = 0;
    http_event_t *event = bpf_map_lookup_elem(&http_scratch_buffer, &zero);
    if (!event) {
        return;
    }

    bpf_memcpy(&event->tuple, tuple, sizeof(conn_tuple_t));
    bpf_memcpy(&event->http, http, sizeof(http_transaction_t));
    http_batch_enqueue(event);
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

static __always_inline bool http_closed(skb_info_t *skb_info) {
    return (skb_info && skb_info->tcp_flags&(TCPHDR_FIN|TCPHDR_RST));
}

// this is merely added here to improve readibility of code.
// HTTP monitoring code is executed in two "contexts":
// * via a socket filter program, which is used for monitoring plain traffic;
// * via a uprobe-based programs, for the purposes of tracing encrypted traffic (SSL, Go TLS, Java TLS etc);
// When code is executed from uprobes, skb_info is null[1].
//
// [1] There is one notable exception that happens when we process uprobes
// triggering the termination of connections. In that particular context we
// "inject" a special skb_info that has the tcp_flags field set to `TCPHDR_FIN`.
static __always_inline bool is_uprobe_context(skb_info_t *skb_info) {
    return skb_info == NULL || (skb_info->data_end == 0 && http_closed(skb_info));
}

// The purpose of http_seen_before is to is to avoid re-processing certain TCP segments.
// We only care about 3 types of segments:
// * A segment with the beginning of a request (packet_type == HTTP_REQUEST);
// * A segment with the beginning of a response (packet_type == HTTP_RESPONSE);
// * A segment with a (FIN|RST) flag set;
static __always_inline bool http_seen_before(http_transaction_t *http, skb_info_t *skb_info, http_packet_t packet_type) {
    if (is_uprobe_context(skb_info) && !http_closed(skb_info)) {
        // The purpose of setting tcp_seq = 0 in the context of uprobe tracing
        // is innocuous for the most part (as this field will almost aways be 0)
        // The only reason we do this here is to *minimize* the chance of a race
        // condition that happens sometimes in the context of uprobe-based tracing:
        //
        // 1) handle_request for c1 (uprobe)
        // 2) socket filter triggers termination code for c1 (server -> FIN -> client)
        // 3) handle_response for c1 (uprobe)
        // 4) socket filter triggers termination code for c1 (client -> FIN -> server)
        //
        // The problem is that 2) and 3) might happen in parallel, and 2) may
        // delete the the eBPF data *before* 4) executes and flushes the data
        // with both request and response information to userspace.
        //
        // Since we check if (skb_info->tcp_seq == HTTP_TERMINATING) evaluates
        // to true before flushing and deleting the eBPF map data, setting it to
        // 0 here gives a chance for the late response to "cancel" the map
        // deletion.
        http->tcp_seq = 0;
        return false;
    }

    if (packet_type != HTTP_REQUEST && packet_type != HTTP_RESPONSE && !http_closed(skb_info)) {
        return false;
    }

    if (http_closed(skb_info)) {
        // Override sequence number with a special sentinel value
        // This is done so we consider
        // Server -> FIN(sequence=x) -> Client
        // And
        // Client -> FIN(sequence=y) -> Server
        // To be the same thing in order to avoid flushing the same transaction twice to userspace
        skb_info->tcp_seq = HTTP_TERMINATING;
    }

    if (http->tcp_seq == skb_info->tcp_seq) {
        return true;
    }

    // Update map entry with latest TCP sequence number
    http->tcp_seq = skb_info->tcp_seq;
    return false;
}

static __always_inline http_transaction_t *http_fetch_state(conn_tuple_t *tuple, http_transaction_t *http, http_packet_t packet_type) {
    if (packet_type == HTTP_PACKET_UNKNOWN) {
        return bpf_map_lookup_elem(&http_in_flight, tuple);
    }

    // We detected either a request or a response
    // In this case we initialize (or fetch) state associated to this tuple
    bpf_map_update_with_telemetry(http_in_flight, tuple, http, BPF_NOEXIST);
    return bpf_map_lookup_elem(&http_in_flight, tuple);
}



// Returns true if the given http transaction should be flushed to the user mode.
// We flush a transaction if:
//   1. We got a new request (packet_type == HTTP_REQUEST) and previously (in the given transaction) we had either a
//      request (http->request_started != 0) or a response (http->response_status_code). This is equivalent to flush
//      a transaction if we have a new request, and the given transaction is not clean.
//   2. We got a new response (packet_type == HTTP_RESPONSE) and the given transaction already contains a response
static __always_inline bool http_should_flush_previous_state(http_transaction_t *http, http_packet_t packet_type) {
    return (packet_type == HTTP_REQUEST && (http->request_started || http->response_status_code)) ||
        (packet_type == HTTP_RESPONSE && http->response_status_code);
}

// http_process is responsible for parsing traffic and emitting events
// representing HTTP transactions.
static __always_inline void http_process(http_event_t *event, skb_info_t *skb_info, __u64 tags) {
    conn_tuple_t *tuple = &event->tuple;
    http_transaction_t *http = &event->http;
    char *buffer = (char *)http->request_fragment;
    http_packet_t packet_type = HTTP_PACKET_UNKNOWN;
    http_method_t method = HTTP_METHOD_UNKNOWN;
    http_parse_data(buffer, &packet_type, &method);

    http = http_fetch_state(tuple, http, packet_type);
    if (!http || http_seen_before(http, skb_info, packet_type)) {
        return;
    }

    if (http_should_flush_previous_state(http, packet_type)) {
        http_batch_enqueue_wrapper(tuple, http);
        bpf_memcpy(http, &event->http, sizeof(http_transaction_t));
    }

    log_debug("http_process: type=%d method=%d", packet_type, method);
    if (packet_type == HTTP_REQUEST) {
        http_begin_request(http, method, buffer);
    } else if (packet_type == HTTP_RESPONSE) {
        http_begin_response(http, buffer);
    }

    http->tags |= tags;

    // Only if we have a (L7/application-layer) payload we update the response_last_seen field
    // This is to prevent things such as keep-alives adding up to the transaction latency
    if (((skb_info && !is_payload_empty(skb_info)) || !skb_info) && http_responding(http)) {
        http->response_last_seen = bpf_ktime_get_ns();
    }

    if (http->tcp_seq == HTTP_TERMINATING) {
        http_batch_enqueue_wrapper(tuple, http);
        // Check a second time to minimize the chance of accidentally deleting a
        // map entry if there is a race with a late response.
        // Please refer to comments in `http_seen_before` for more context.
        if (http->tcp_seq == HTTP_TERMINATING) {
            bpf_map_delete_elem(&http_in_flight, tuple);
        }
    }
}

// this function is called by the socket-filter program to decide whether or not we should inspect
// the contents of a certain packet, in order to avoid the cost of processing packets that are not
// of interest such as empty ACKs, UDP data or encrypted traffic.
static __always_inline bool http_allow_packet(conn_tuple_t *tuple, struct __sk_buff* skb, skb_info_t *skb_info) {
    // we're only interested in TCP traffic
    if (!(tuple->metadata&CONN_TYPE_TCP)) {
        return false;
    }

    bool empty_payload = skb_info->data_off == skb->len;
    if (empty_payload || tuple->sport == HTTPS_PORT || tuple->dport == HTTPS_PORT) {
        // if the payload data is empty or encrypted packet, we only
        // process it if the packet represents a TCP termination
        return skb_info->tcp_flags&(TCPHDR_FIN|TCPHDR_RST);
    }

    return true;
}

SEC("socket/http_filter")
int socket__http_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;
    http_event_t event;
    bpf_memset(&event, 0, sizeof(http_event_t));

    if (!fetch_dispatching_arguments(&event.tuple, &skb_info)) {
        log_debug("http_filter failed to fetch arguments for tail call");
        return 0;
    }

    if (!http_allow_packet(&event.tuple, skb, &skb_info)) {
        return 0;
    }
    normalize_tuple(&event.tuple);

    read_into_buffer_skb((char *)event.http.request_fragment, skb, skb_info.data_off);
    http_process(&event, &skb_info, NO_TAGS);
    return 0;
}

SEC("uprobe/http_process")
int uprobe__http_process(struct pt_regs *ctx) {
    const __u32 zero = 0;
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    http_event_t event;
    bpf_memset(&event, 0, sizeof(http_event_t));
    bpf_memcpy(&event.tuple, &args->tup, sizeof(conn_tuple_t));
    read_into_user_buffer_http(event.http.request_fragment, args->buffer_ptr);
    http_process(&event, NULL, args->tags);
    http_batch_flush(ctx);

    return 0;
}

SEC("uprobe/http_termination")
int uprobe__http_termination(struct pt_regs *ctx) {
    const __u32 zero = 0;
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    http_event_t event;
    bpf_memset(&event, 0, sizeof(http_event_t));
    bpf_memcpy(&event.tuple, &args->tup, sizeof(conn_tuple_t));
    skb_info_t skb_info = {0};
    skb_info.tcp_flags |= TCPHDR_FIN;
    http_process(&event, &skb_info, NO_TAGS);
    http_batch_flush(ctx);

    return 0;
}

#endif
