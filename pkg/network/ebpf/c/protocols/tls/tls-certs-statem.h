#ifndef __TLS_CERTS_STATEM_H
#define __TLS_CERTS_STATEM_H


#include "ktypes.h"
#include "defs.h"
#include "bpf_builtins.h"
#include "bpf_tracing.h"
#include "bpf_bypass.h"


#include "bpf_telemetry.h"
#include "tls-certs-maps.h"

// This file performs ssl_certs_statem_args bookkeeping for functions that enter the SSL state machine

SEC("uprobe/SSL_do_handshake")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_do_handshake, void *ssl_ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_do_handshake: pid_tgid=%llx ssl_ctx=%p", pid_tgid, ssl_ctx);
    bpf_map_update_with_telemetry(ssl_certs_statem_args, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_do_handshake")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_do_handshake) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/SSL_do_handshake: pid_tgid=%llx", pid_tgid);
    bpf_map_delete_elem(&ssl_certs_statem_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_read")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_read, void *ssl_ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_read: pid_tgid=%llx ssl_ctx=%p", pid_tgid, ssl_ctx);
    bpf_map_update_with_telemetry(ssl_certs_statem_args, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_read")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_read) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/SSL_read: pid_tgid=%llx", pid_tgid);
    bpf_map_delete_elem(&ssl_certs_statem_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_read_ex")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_read_ex, void *ssl_ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_read_ex: pid_tgid=%llx ssl_ctx=%p", pid_tgid, ssl_ctx);
    bpf_map_update_with_telemetry(ssl_certs_statem_args, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_read_ex")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_read_ex) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/SSL_read_ex: pid_tgid=%llx", pid_tgid);
    bpf_map_delete_elem(&ssl_certs_statem_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_write")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_write, void *ssl_ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_write: pid_tgid=%llx ssl_ctx=%p", pid_tgid, ssl_ctx);
    bpf_map_update_with_telemetry(ssl_certs_statem_args, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_write")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_write) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/SSL_write: pid_tgid=%llx", pid_tgid);
    bpf_map_delete_elem(&ssl_certs_statem_args, &pid_tgid);
    return 0;
}


SEC("uprobe/SSL_write_ex")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_write_ex, void *ssl_ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_write_ex: pid_tgid=%llx ssl_ctx=%p", pid_tgid, ssl_ctx);
    bpf_map_update_with_telemetry(ssl_certs_statem_args, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_write_ex")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_write_ex) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/SSL_write_ex: pid_tgid=%llx", pid_tgid);
    bpf_map_delete_elem(&ssl_certs_statem_args, &pid_tgid);
    return 0;
}


#endif //__TLS_CERTS_STATEM_H