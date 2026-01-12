#ifndef __TLS_CERTS_STATEM_H
#define __TLS_CERTS_STATEM_H

#ifndef COMPILE_PREBUILT

#include "ktypes.h"
#include "defs.h"
#include "bpf_builtins.h"
#include "bpf_tracing.h"
#include "bpf_bypass.h"


#include "bpf_telemetry.h"
#include "tls-certs-maps.h"

// This file performs ssl_certs_statem_args bookkeeping for functions that enter the SSL state machine


static __always_inline void enter_state_machine(const char *probe, void *ssl_ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("%s: pid=%u tgid=%u", probe, PID_FROM(pid_tgid), TGID_FROM(pid_tgid));

    bpf_map_update_with_telemetry(ssl_certs_statem_args, &pid_tgid, &ssl_ctx, BPF_ANY);
}

static __always_inline void exit_state_machine(const char *probe) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("%s: pid=%u tgid=%u", probe, PID_FROM(pid_tgid), TGID_FROM(pid_tgid));
    bpf_map_delete_elem(&ssl_certs_statem_args, &pid_tgid);
}

SEC("uprobe/SSL_do_handshake")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_do_handshake, void *ssl_ctx) {
    enter_state_machine("uprobe/SSL_do_handshake", ssl_ctx);
    return 0;
}

SEC("uretprobe/SSL_do_handshake")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_do_handshake) {
    exit_state_machine("uretprobe/SSL_do_handshake");
    return 0;
}

SEC("uprobe/SSL_read")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_read, void *ssl_ctx) {
    enter_state_machine("uprobe/SSL_read", ssl_ctx);
    return 0;
}

SEC("uretprobe/SSL_read")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_read) {
    exit_state_machine("uretprobe/SSL_read");
    return 0;
}

SEC("uprobe/SSL_read_ex")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_read_ex, void *ssl_ctx) {
    enter_state_machine("uprobe/SSL_read_ex", ssl_ctx);
    return 0;
}

SEC("uretprobe/SSL_read_ex")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_read_ex) {
    exit_state_machine("uretprobe/SSL_read_ex");
    return 0;
}

SEC("uprobe/SSL_write")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_write, void *ssl_ctx) {
    enter_state_machine("uprobe/SSL_write", ssl_ctx);
    return 0;
}

SEC("uretprobe/SSL_write")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_write) {
    exit_state_machine("uretprobe/SSL_write");
    return 0;
}


SEC("uprobe/SSL_write_ex")
int BPF_BYPASSABLE_UPROBE(uprobe__SSL_write_ex, void *ssl_ctx) {
    enter_state_machine("uprobe/SSL_write_ex", ssl_ctx);
    return 0;
}

SEC("uretprobe/SSL_write_ex")
int BPF_BYPASSABLE_URETPROBE(uretprobe__SSL_write_ex) {
    exit_state_machine("uretprobe/SSL_write_ex");
    return 0;
}

#endif //COMPILE_PREBUILT


#endif //__TLS_CERTS_STATEM_H
