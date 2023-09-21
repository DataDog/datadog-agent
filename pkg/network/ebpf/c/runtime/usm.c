#include "bpf_tracing.h"
#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#include "ktypes.h"
#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#endif

#include "ip.h"
#include "ipv6.h"
#include "sock.h"
#include "port_range.h"

#include "protocols/classification/dispatcher-helpers.h"
#include "protocols/http/buffer.h"
#include "protocols/http/http.h"
#include "protocols/http2/decoding.h"
#include "protocols/kafka/kafka-parsing.h"
#include "protocols/tls/java/erpc_dispatcher.h"
#include "protocols/tls/java/erpc_handlers.h"
#include "protocols/tls/go-tls-types.h"
#include "protocols/tls/go-tls-goid.h"
#include "protocols/tls/go-tls-location.h"
#include "protocols/tls/go-tls-conn.h"
#include "protocols/tls/https.h"
#include "protocols/tls/native-tls.h"
#include "protocols/tls/tags-types.h"

// The entrypoint for all packets classification & decoding in universal service monitoring.
SEC("socket/protocol_dispatcher")
int socket__protocol_dispatcher(struct __sk_buff *skb) {
    protocol_dispatcher_entrypoint(skb);
    return 0;
}

// This entry point is needed to bypass a memory limit on socket filters
// See: https://datadoghq.atlassian.net/wiki/spaces/NET/pages/2326855913/HTTP#Known-issues
SEC("socket/protocol_dispatcher_kafka")
int socket__protocol_dispatcher_kafka(struct __sk_buff *skb) {
    dispatch_kafka(skb);
    return 0;
}

SEC("kprobe/tcp_sendmsg")
int BPF_KPROBE(kprobe__tcp_sendmsg, struct sock *sk) {
    log_debug("kprobe/tcp_sendmsg: sk=%llx\n", sk);
    // map connection tuple during SSL_do_handshake(ctx)
    map_ssl_ctx_to_sock(sk);

    return 0;
}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb(struct pt_regs* ctx) {
    log_debug("tracepoint/net/netif_receive_skb\n");
    // flush batch to userspace
    // because perf events can't be sent from socket filter programs
    http_batch_flush(ctx);
    http2_batch_flush(ctx);
    kafka_batch_flush(ctx);
    return 0;
}

// GO TLS PROBES

// func (c *Conn) Write(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Write")
int uprobe__crypto_tls_Conn_Write(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = pid_tgid >> 32;
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-write] no offsets data in map for pid %d\n", pid);
        return 0;
    }

    // Read the PID and goroutine ID to make the partial call key
    go_tls_function_args_key_t call_key = {0};
    call_key.pid = pid;
    if (read_goroutine_id(ctx, &od->goroutine_id, &call_key.goroutine_id)) {
        log_debug("[go-tls-write] failed reading go routine id for pid %d\n", pid);
        return 0;
    }

    // Read the parameters to make the partial call data
    // (since the parameters might not be live by the time the return probe is hit).
    go_tls_write_args_data_t call_data = {0};
    if (read_location(ctx, &od->write_conn_pointer, sizeof(call_data.conn_pointer), &call_data.conn_pointer)) {
        log_debug("[go-tls-write] failed reading conn pointer for pid %d\n", pid);
        return 0;
    }

    if (read_location(ctx, &od->write_buffer.ptr, sizeof(call_data.b_data), &call_data.b_data)) {
        log_debug("[go-tls-write] failed reading buffer pointer for pid %d\n", pid);
        return 0;
    }

    if (read_location(ctx, &od->write_buffer.len, sizeof(call_data.b_len), &call_data.b_len)) {
        log_debug("[go-tls-write] failed reading buffer length for pid %d\n", pid);
        return 0;
    }

    bpf_map_update_elem(&go_tls_write_args, &call_key, &call_data, BPF_ANY);
    return 0;
}

// func (c *Conn) Write(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Write/return")
int uprobe__crypto_tls_Conn_Write__return(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = pid_tgid >> 32;
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-write-return] no offsets data in map for pid %d\n", pid);
        return 0;
    }

    // Read the PID and goroutine ID to make the partial call key
    go_tls_function_args_key_t call_key = {0};
    call_key.pid = pid;

    if (read_goroutine_id(ctx, &od->goroutine_id, &call_key.goroutine_id)) {
        log_debug("[go-tls-write-return] failed reading go routine id for pid %d\n", pid);
        return 0;
    }

    uint64_t bytes_written = 0;
    if (read_location(ctx, &od->write_return_bytes, sizeof(bytes_written), &bytes_written)) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] failed reading write return bytes location for pid %d\n", pid);
        return 0;
    }

    if (bytes_written <= 0) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] write returned non-positive for amount of bytes written for pid: %d\n", pid);
        return 0;
    }

    uint64_t err_ptr = 0;
    if (read_location(ctx, &od->write_return_error, sizeof(err_ptr), &err_ptr)) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] failed reading write return error location for pid %d\n", pid);
        return 0;
    }

    // check if err != nil
    if (err_ptr != 0) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] error in write for pid %d: data will be ignored\n", pid);
        return 0;
    }

    go_tls_write_args_data_t *call_data_ptr = bpf_map_lookup_elem(&go_tls_write_args, &call_key);
    if (call_data_ptr == NULL) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] no write information in write-return for pid %d\n", pid);
        return 0;
    }

    conn_tuple_t *t = conn_tup_from_tls_conn(od, (void*)call_data_ptr->conn_pointer, pid_tgid);
    if (t == NULL) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        return 0;
    }

    log_debug("[go-tls-write] processing %s\n", call_data_ptr->b_data);
    char *buffer_ptr = (char*)call_data_ptr->b_data;
    bpf_map_delete_elem(&go_tls_write_args, &call_key);
    tls_process(ctx, t, buffer_ptr, bytes_written, GO);
    return 0;
}

// func (c *Conn) Read(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Read")
int uprobe__crypto_tls_Conn_Read(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = pid_tgid >> 32;
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-read] no offsets data in map for pid %d\n", pid_tgid >> 32);
        return 0;
    }

    // Read the PID and goroutine ID to make the partial call key
    go_tls_function_args_key_t call_key = {0};
    call_key.pid = pid;
    if (read_goroutine_id(ctx, &od->goroutine_id, &call_key.goroutine_id)) {
        log_debug("[go-tls-read] failed reading go routine id for pid %d\n", pid_tgid >> 32);
        return 0;
    }

    // Read the parameters to make the partial call data
    // (since the parameters might not be live by the time the return probe is hit).
    go_tls_read_args_data_t call_data = {0};
    if (read_location(ctx, &od->read_conn_pointer, sizeof(call_data.conn_pointer), &call_data.conn_pointer)) {
        log_debug("[go-tls-read] failed reading conn pointer for pid %d\n", pid_tgid >> 32);
        return 0;
    }
    if (read_location(ctx, &od->read_buffer.ptr, sizeof(call_data.b_data), &call_data.b_data)) {
        log_debug("[go-tls-read] failed reading buffer pointer for pid %d\n", pid_tgid >> 32);
        return 0;
    }

    bpf_map_update_elem(&go_tls_read_args, &call_key, &call_data, BPF_ANY);
    return 0;
}

// func (c *Conn) Read(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Read/return")
int uprobe__crypto_tls_Conn_Read__return(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = pid_tgid >> 32;
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-read-return] no offsets data in map for pid %d\n", pid);
        return 0;
    }

    // On 4.14 kernels we suffered from a verifier issue, that lost track on `call_key` and failed later when accessing
    // to it. The workaround was to delay its creation, so we're getting the goroutine separately.
    __s64 goroutine_id = 0;
    // Read the PID and goroutine ID to make the partial call key
    if (read_goroutine_id(ctx, &od->goroutine_id, &goroutine_id)) {
        log_debug("[go-tls-read-return] failed reading go routine id for pid %d\n", pid);
        return 0;
    }

    go_tls_function_args_key_t call_key = {0};
    call_key.pid = pid;
    call_key.goroutine_id = goroutine_id;

    go_tls_read_args_data_t* call_data_ptr = bpf_map_lookup_elem(&go_tls_read_args, &call_key);
    if (call_data_ptr == NULL) {
        log_debug("[go-tls-read-return] no read information in read-return for pid %d\n", pid);
        return 0;
    }

    uint64_t bytes_read = 0;
    if (read_location(ctx, &od->read_return_bytes, sizeof(bytes_read), &bytes_read)) {
        log_debug("[go-tls-read-return] failed reading return bytes location for pid %d\n", pid);
        bpf_map_delete_elem(&go_tls_read_args, &call_key);
        return 0;
    }

    // Errors like "EOF" of "unexpected EOF" can be treated as no error by the hooked program.
    // Therefore, if we choose to ignore data if read had returned these errors we may have accuracy issues.
    // For now for success validation we chose to check only the amount of bytes read
    // and make sure it's greater than zero.
    if (bytes_read <= 0) {
        log_debug("[go-tls-read-return] read returned non-positive for amount of bytes read for pid: %d\n", pid);
        bpf_map_delete_elem(&go_tls_read_args, &call_key);
        return 0;
    }

    conn_tuple_t* t = conn_tup_from_tls_conn(od, (void*) call_data_ptr->conn_pointer, pid_tgid);
    if (t == NULL) {
        bpf_map_delete_elem(&go_tls_read_args, &call_key);
        return 0;
    }

    char *buffer_ptr = (char*)call_data_ptr->b_data;
    bpf_map_delete_elem(&go_tls_read_args, (go_tls_function_args_key_t*)&call_key);

    tls_process(ctx, t, buffer_ptr, bytes_read, GO);
    return 0;
}

// func (c *Conn) Close(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Close")
int uprobe__crypto_tls_Conn_Close(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-close] no offsets data in map for pid %d\n", pid_tgid >> 32);
        return 0;
    }

    // Read the PID and goroutine ID to make the partial call key
    go_tls_function_args_key_t call_key = {0};
    call_key.pid = pid_tgid >> 32;
    if (read_goroutine_id(ctx, &od->goroutine_id, &call_key.goroutine_id) == 0) {
        bpf_map_delete_elem(&go_tls_read_args, &call_key);
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
    }

    void* conn_pointer = NULL;
    if (read_location(ctx, &od->close_conn_pointer, sizeof(conn_pointer), &conn_pointer)) {
        log_debug("[go-tls-close] failed reading close conn pointer for pid %d\n", pid_tgid >> 32);
        return 0;
    }

    conn_tuple_t* t = conn_tup_from_tls_conn(od, conn_pointer, pid_tgid);
    if (t == NULL) {
        log_debug("[go-tls-close] failed getting conn tup from tls conn for pid %d\n", pid_tgid >> 32);
        return 0;
    }

    // Clear the element in the map since this connection is closed
    bpf_map_delete_elem(&conn_tup_by_go_tls_conn, &conn_pointer);

    // tls_finish can launch a tail call, thus cleanup should be done before.
    tls_finish(ctx, t);
    return 0;
}

static __always_inline void* get_tls_base(struct task_struct* task) {
#if defined(__TARGET_ARCH_x86)
    // X86 (RUNTIME & CO-RE)
    return (void *)BPF_CORE_READ(task, thread.fsbase);
#elif defined(__TARGET_ARCH_arm64)
#if defined(COMPILE_RUNTIME)
    // ARM64 (RUNTIME)
#if LINUX_VERSION_CODE >= KERNEL_VERSION(5, 5, 0)
    return (void *)BPF_CORE_READ(task, thread.uw.tp_value);
#else
    // This branch (kernel < 5.5) won't ever be executed, but is needed for
    // for the runtime compilation/program load to work in older kernels.
    return NULL;
#endif
#else
    // ARM64 (CO-RE)
    // Note that all Kernels currently supported by GoTLS monitoring (>= 5.5) do
    // have the field below, but if we don't check for its existence the program
    // *load* may fail in older Kernels, even if GoTLS monitoring is disabled.
    if (bpf_core_field_exists(task->thread.uw)) {
        return (void *)BPF_CORE_READ(task, thread.uw.tp_value);
    } else {
        return NULL;
    }
#endif
#else
    #error "Unsupported platform"
#endif
}

char _license[] SEC("license") = "GPL";
