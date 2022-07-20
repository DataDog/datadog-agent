#ifndef __TLS_H
#define __TLS_H

#include "tracer.h"
#include "classifier.h"
#include "tags.h"
#include "tls-types.h"
#include "tls-maps.h"
#include "port_range.h"
#include "http.h"
#include "classifier-telemetry.h"
#include "bpf_endian.h"

#include <uapi/linux/ptrace.h>

struct bpf_map_def SEC("maps/proto_args") proto_args = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u32),
    .value_size = sizeof(proto_args_t),
    .max_entries = 1,
};


static __always_inline int is_valid_tls_app(u8 app) {
    return (app == TLS_CHANGE_CIPHER) || (app == TLS_ALERT) || (app == TLS_HANDSHAKE) || (app == TLS_APPLICATION_DATA);
}

static __always_inline int is_valid_tls_version(__u16 version) {
    return (version == SSL_VERSION20) || (version == SSL_VERSION30) || (version == TLS_VERSION10) || (version == TLS_VERSION11) || (version == TLS_VERSION12) || (version == TLS_VERSION13);
}

static __always_inline int sane_payload_length(__u8 app, __u16 tls_len, __u16 skb_len, u32 offset) {
    if (app != TLS_APPLICATION_DATA)
        return 1;

    if (skb_len > (tls_len+offset+TLS_RECORD_LEN)) {
        log_debug("skb may contain multiple TLS payload\n");
        log_debug("skb_len: %d, payload: %d\n", skb_len, tls_len+offset+TLS_RECORD_LEN);
        return 0;
    }

    return 1;
}

static __always_inline int is_tls(struct __sk_buff* skb, u32 offset) {
    if (skb->len - offset <= 0)
        return 0;

    if (skb->len - offset < TLS_HEADER_SIZE)
        return 0;

    __u8 app = load_byte(skb, offset);
    if (!is_valid_tls_app(app))
        return 0;

    __u16 version = load_half(skb, offset + 1);
    if (!is_valid_tls_version(version))
        return 0;

    __u16 length = load_half(skb, offset + 3);
    if (length > MAX_TLS_FRAGMENT_LENGTH)
        return 0;

    if (!sane_payload_length(app, length, skb->len, offset))
        return 0;

    return 1;
}

static __always_inline void parse_tls_server_hello(tls_session_t* tls, struct __sk_buff *skb, u32 offset) {
    tls->cipher_suite = load_half(skb, offset+45);
    // TODO: parse extensions to find if tls 1.3
}

static __always_inline void handle_tls_handshake(tls_session_t* tls, struct __sk_buff* skb, u32 offset) {
    __u8 handshake = load_byte(skb, offset+5);

    if (handshake == SERVER_HELLO) {
        tls->state |= STATE_HELLO_SERVER;
        parse_tls_server_hello(tls, skb, offset);
    } else if (handshake == CLIENT_HELLO) {
        tls->state |= STATE_HELLO_CLIENT;
    } else if (handshake == CERTIFICATE) {
        tls->state |= STATE_SHARE_CERTIFICATE;
    }
}

static __always_inline void handle_tls_app_data(tls_session_t *tls) {
    if ((tls->state & STATE_HELLO_CLIENT) && (tls->state & STATE_HELLO_SERVER)) {
        if ((tls->version != TLS_VERSION13) && (tls->state & STATE_SHARE_CERTIFICATE)) {
            tls->state |= STATE_APPLICATION_DATA;
        }
        tls->state |= STATE_APPLICATION_DATA;

        return;
    }
}

static __always_inline void transition_session_state(tls_session_t* tls, struct __sk_buff* skb, u32 offset) {
    tls_record_t record;

    // read the tls record
    record.app = load_byte(skb, offset);
    record.version = load_half(skb, offset+1);
    record.length = load_half(skb, offset+3);

    if (skb->len < (record.length + TLS_RECORD_LEN + offset))
        return;

    if (record.app == TLS_HANDSHAKE) {
        handle_tls_handshake(tls, skb, offset);
    } else if (record.app == TLS_APPLICATION_DATA) {
//        handle_tls_app_data(tls);
        tls->state |= STATE_APPLICATION_DATA;
    }
}

static __always_inline void check_set_done(tls_session_t* tls, conn_tuple_t* tup) {
    if (tls->state & STATE_APPLICATION_DATA) {
        tls->info.done = 1;
        add_tags_tuple(tup, TLS);
    }
}

/*
   proto_tls() parsing TLS packets until
    o we see TLS_APPLICATION_DATA packets
    o TLS_MAX_PACKET_CLASSIFIER is reached
 */
SEC("socket/proto_tls")
int socket__proto_tls(struct __sk_buff* skb) {
    proto_args_t *args;
    skb_info_t *skb_info;
    conn_tuple_t *tup;

    u32 cpu = bpf_get_smp_processor_id();
    args = bpf_map_lookup_elem(&proto_args, &cpu);
    if (args == NULL)
        return 0;

    skb_info = &args->skb_info;
    tup = &args->tup;

    tls_session_t *tls = NULL;
    tls_session_t new_entry = { 0 };
    bpf_map_update_elem(&proto_in_flight, tup, &new_entry, BPF_NOEXIST);
    tls = bpf_map_lookup_elem(&proto_in_flight, tup);
    if (tls == NULL)
        return 0;

    /* cnx classified or not */
    if (tls->packets > TLS_MAX_PACKET_CLASSIFIER) {
        tls->info.failed = 1;
        return 0;
    }
    tls->packets++;

    transition_session_state(tls, skb, skb_info->data_off);
    check_set_done(tls, tup);

    return 0;
}

#endif
