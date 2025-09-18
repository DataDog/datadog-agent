#ifndef __TLS_CERTS_H
#define __TLS_CERTS_H

#include "ktypes.h"
#include "bpf_builtins.h"
#include "bpf_tracing.h"
#include "bpf_bypass.h"


#include "bpf_telemetry.h"
#include "tls-certs-statem.h"
#include "tls-certs-maps.h"
#include "tls-certs-parser.h"

static __always_inline void SSL_report_cert(conn_stats_ts_t *stats) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    void **ssl_ctx_mapval = bpf_map_lookup_elem(&ssl_certs_statem_args, &pid_tgid);
    // we are not inside SSL_do_handshake, skip
    if (ssl_ctx_mapval == NULL) {
        return;
    }
    void *ssl_ctx = *ssl_ctx_mapval;

    ssl_handshake_state_t *state = bpf_map_lookup_elem(&ssl_handshake_state, &ssl_ctx);
    if (state == NULL) {
        return;
    }

    // SSL_add_cert has not been called, the cert is not ready yet
    if (!state->cert_id) {
        return;
    }
    cert_id_t cert_id = state->cert_id;
    stats->cert_id = cert_id;

    // we don't need the handshake state anymore now that we've used it
    bpf_map_delete_elem(&ssl_handshake_state, &ssl_ctx);

    log_debug("SSL_report_cert: pid_tgid=%llx reported cert id=%x", pid_tgid, cert_id);
}


static __always_inline void SSL_add_cert(void *ssl_ctx, data_t data) {
    cert_t cert = {0};
    if (parse_cert(data, &cert)) {
        log_debug("SSL_add_cert failed to parse the cert");


        for (int i = 0; i < 16; i++) {
            __u64 buf = 0;
            if (data_read(&buf, &data, 4)) {
                log_debug("SSL_add_cert failed to read the cert");
                return;
            }
            log_debug("SSL_add_cert: %llx", buf);
        }

        return;
    }

    ssl_handshake_state_t empty_state = {0};
    bpf_map_update_with_telemetry(ssl_handshake_state, &ssl_ctx, &empty_state, BPF_NOEXIST, -EEXIST);
    
    ssl_handshake_state_t *state =  bpf_map_lookup_elem(&ssl_handshake_state, &ssl_ctx);
    if (state == NULL) {
        log_debug("SSL_add_cert: couldn't find handshake state");
        return;
    }

    if (!cert.is_ca) {
        __u64 timestamp = bpf_ktime_get_ns();
        state->timestamp = timestamp;

        state->cert_item.serial = cert.serial;
        state->cert_item.domain = cert.domain;
        state->cert_item.validity = cert.validity;

        state->cert_id = cert.cert_id;

        // reuse the stack space we allocated for empty_state
        cert_item_t *scratch = &empty_state.cert_item;
        *scratch = state->cert_item;
        bpf_map_update_with_telemetry(ssl_cert_info, &cert.cert_id, scratch, BPF_ANY);
    }
}

SEC("uprobe/i2d_X509")
int BPF_BYPASSABLE_UPROBE(uprobe__i2d_X509) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    __u8 **out = (__u8**)PT_REGS_PARM2(ctx);
    if (!out) {
        // they're just testing the length of the cert by passing in a null pointer, skip
        return 0;
    }
    log_debug("uprobe/i2d_X509: pid_tgid=%llx out=%p", pid_tgid, out);

    // i2d_X509 has two behaviors:
    // 1. if *out is NULL, it will allocate a new buffer for the output
    // 2. if *out is not NULL, it will use the buffer pointed to by *out, AND overwrite the pointer so
    //    that it points past the end of what it wrote
    // out_deref stores *out so we can handle these cases
    __u8 *out_deref = 0;
    int err = bpf_probe_read_user(&out_deref, sizeof(u8*), out);
    if (err) {
        log_debug("i2d_X509 failed to read *out at %p: %d", out, err);
        return 0;
    }

    i2d_X509_args_t args = {
        .out = out,
        .out_deref = out_deref,
    };
    bpf_map_update_with_telemetry(ssl_certs_i2d_X509_args, &pid_tgid, &args, BPF_ANY);

    return 0;
}


SEC("uretprobe/i2d_X509")
int BPF_BYPASSABLE_URETPROBE(uretprobe__i2d_X509) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/i2d_X509: pid_tgid=%llx", pid_tgid);

    int data_len = (int)PT_REGS_RC(ctx);
    if (data_len < 0) {
        log_debug("uretprobe/i2d_X509: i2d_X509 failed with err=%d", data_len);
        return 0;
    }

    i2d_X509_args_t* args = bpf_map_lookup_elem(&ssl_certs_i2d_X509_args, &pid_tgid);
    if (!args) {
        return 0;
    }
    log_debug("uretprobe/i2d_X509: data_len=%d", data_len);

    void **ssl_ctx_mapval = bpf_map_lookup_elem(&ssl_certs_statem_args, &pid_tgid);
    // we are not inside the SSL state machine, skip
    if (!ssl_ctx_mapval) {
        return 0;
    }

    __u8 **out = args->out;
    __u8 *out_deref = args->out_deref;
    if (!out_deref) {
        int err = bpf_probe_read_user(&out_deref, sizeof(u8*), out);
        if (err) {
            log_debug("i2d_X509 failed to read the data pointer %p: %d", out, err);
            return 0;
        }
    }

    bpf_map_delete_elem(&ssl_certs_i2d_X509_args, &pid_tgid);

    data_t data = { out_deref, out_deref + data_len };
    SSL_add_cert(*ssl_ctx_mapval, data);

    return 0;
}


SEC("raw_tracepoint/sched_process_exit")
int raw_tracepoint__sched_process_exit_ssl_cert(void *ctx) {
    CHECK_BPF_PROGRAM_BYPASSED()

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("raw_tracepoint/sched_process_exit: pid_tgid=%llx", pid_tgid);

    bpf_map_delete_elem(&ssl_certs_statem_args, &pid_tgid);
    bpf_map_delete_elem(&ssl_certs_i2d_X509_args, &pid_tgid);

    return 0;
}


#endif //__TLS_CERTS_H
