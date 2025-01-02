#include "bpf_tracing.h"
#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "bpf_metadata.h"
#include "bpf_bypass.h"

#include "ktypes.h"
#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#endif

#include "ip.h"
#include "ipv6.h"
#include "sock.h"
#include "port_range.h"
#include "pid_tgid.h"

#include "protocols/classification/dispatcher-helpers.h"
#include "protocols/classification/dispatcher-helpers-kprobes.h"
#include "protocols/flush.h"
#include "protocols/http/buffer.h"
#include "protocols/http/http.h"
#include "protocols/http2/decoding.h"
#include "protocols/http2/decoding-tls.h"
#include "protocols/http2/decoding-kprobe.h"
#include "protocols/kafka/kafka-parsing.h"
#include "protocols/postgres/decoding.h"
#include "protocols/redis/decoding.h"
#include "protocols/sockfd-probes.h"
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

// This entry point is needed to bypass stack limit errors if `is_kafka()` is called
// from the regular TLS dispatch entrypoint.
SEC("uprobe/tls_protocol_dispatcher_kafka")
int uprobe__tls_protocol_dispatcher_kafka(struct pt_regs *ctx) {
    tls_dispatch_kafka(ctx);
    return 0;
};

SEC("kprobe/protocol_dispatcher_kafka")
int kprobe__protocol_dispatcher_kafka(struct pt_regs *ctx) {
    kprobe_dispatch_kafka(ctx);
    return 0;
};

struct iov_iter___new {
    u8 iter_type;
    void *ubuf;
};

struct iov_iter___old {
    unsigned int type;
};

struct msghdr___old {
    struct iov_iter___old msg_iter;
};

struct msghdr___new {
    struct iov_iter___new msg_iter;
};

SEC("kprobe/tcp_recvmsg")
int BPF_KPROBE(kprobe__tcp_recvmsg, struct sock *sk, struct msghdr *msg, size_t len, int flags) {
#ifdef COMPILE_CORE
    u64 pid_tgid = bpf_get_current_pid_tgid();

    log_debug("kprobe/tcp_recvmsg: sk=%lx msghdr=%lx!\n", (unsigned long)sk, (unsigned long)msg);
    log_debug("kprobe/tcp_recvmsg: len=%lu\n", len);

    if (bpf_core_field_exists(((struct msghdr___old *)msg)->msg_iter.type)) {
        // 5.10
        unsigned int type;
        BPF_CORE_READ_INTO(&type, (struct msghdr___old *)msg, msg_iter.type);
        log_debug("kprobe/tcp_recvmsg: type=%u", type);
    } else {
        u8 iter_type;
        BPF_CORE_READ_INTO(&iter_type, msg, msg_iter.iter_type);
        log_debug("kprobe/tcp_recvmsg: iter_type=%u", iter_type);
    }

    void *ubuf;

    if (bpf_core_field_exists(((struct msghdr___new *)msg)->msg_iter.ubuf)) {
        BPF_CORE_READ_INTO(&ubuf, (struct msghdr___new *)msg, msg_iter.ubuf);
    } else {
        size_t count = 0;

        BPF_CORE_READ_INTO(&ubuf, msg, msg_iter.iov, iov_base);
        BPF_CORE_READ_INTO(&count, msg, msg_iter.count);
        log_debug("kprobe/tcp_recvmsg: count=%zu", count);
    }

    log_debug("kprobe/tcp_recvmsg: ubuf=%lx", (unsigned long)ubuf);

    tcp_kprobe_state_t state = {
        .sock = sk,
        .buffer = ubuf,
    };
    bpf_map_update_with_telemetry(tcp_kprobe_state, &pid_tgid, &state, BPF_ANY);
#endif /* COMPILE_CORE */

    // map connection tuple during SSL_do_handshake(ctx)
    map_ssl_ctx_to_sock(sk);

    return 0;
}

SEC("kretprobe/tcp_recvmsg")
int BPF_KRETPROBE(kretprobe__tcp_recvmsg, int ret) {
    log_debug("kretprobe/tcp_recvmsg ret=%d", ret);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    tcp_kprobe_state_t *state = bpf_map_lookup_elem(&tcp_kprobe_state, &pid_tgid);
    if (!state) {
        log_debug("kretprobe/tcp_recvmsg no state");
        return 0;
    }

    tcp_kprobe_state_t statecopy = *state;
    bpf_map_delete_elem(&tcp_kprobe_state, &pid_tgid);

    if (ret > 0) {
        u64 data0 = 0;
        u64 data1 = 0;
        bpf_probe_read_user(&data0, sizeof(data0), statecopy.buffer);
        bpf_probe_read_user(&data1, sizeof(data1), statecopy.buffer + sizeof(data1));
        log_debug("recvmsg data0=%llx", bpf_be64_to_cpu(data0));
        log_debug("recvmsg data1=%llx", bpf_be64_to_cpu(data1));

        kprobe_protocol_dispatcher_entrypoint(ctx, statecopy.sock, statecopy.buffer, ret, true);
    }

    return 0;
}

SEC("kprobe/tcp_sendmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_sendmsg_socket_filter, struct sock *sk) {
    log_debug("kprobe/tcp_sendmsg: sk=%p", sk);
    // map connection tuple during SSL_do_handshake(ctx)
    map_ssl_ctx_to_sock(sk);

    return 0;
}

SEC("kprobe/tcp_sendmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_sendmsg_kprobe, struct sock *sk, struct msghdr *msg, size_t size) {
    log_debug("kprobe/tcp_sendmsg: sk=%lx msghdr=%lx!\n", (unsigned long)sk, (unsigned long)msg);
    log_debug("kprobe/tcp_sendmsg: size=%lu\n", size);

    // map connection tuple during SSL_do_handshake(ctx)
    map_ssl_ctx_to_sock(sk);

#ifdef COMPILE_CORE
    bool iov_ubuf = false;

    if (bpf_core_field_exists(((struct msghdr___old *)msg)->msg_iter.type)) {
        // 5.10
        unsigned int type;
        BPF_CORE_READ_INTO(&type, (struct msghdr___old *)msg, msg_iter.type);
        log_debug("kprobe/tcp_sendmsg: type=%u", type);
    } else {
        u8 iter_type;
        BPF_CORE_READ_INTO(&iter_type, msg, msg_iter.iter_type);
        log_debug("kprobe/tcp_sendmsg: iter_type=%u", iter_type);
        if (iter_type == 1) {
            iov_ubuf = true;
        }
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 *splicing = bpf_map_lookup_elem(&tcp_splicing, &pid_tgid);
    if (splicing) {
        log_debug("kprobe/tcp_sendmsg: ignore due to splice");
        return 0;
    }


    void *ubuf;

    if (bpf_core_field_exists(((struct msghdr___new *)msg)->msg_iter.ubuf)) {
        BPF_CORE_READ_INTO(&ubuf, (struct msghdr___new *)msg, msg_iter.ubuf);

        if (iov_ubuf) {
            struct iovec *iov = ubuf;
            unsigned long iov_len = 0;

            log_debug("kprobe/tcp_sendmsg iov=%lx", (unsigned long)iov);

            bpf_probe_read_kernel(&ubuf, sizeof(ubuf), &iov->iov_base);
            bpf_probe_read_kernel(&iov_len, sizeof(iov_len), &iov->iov_len);

            log_debug("kprobe/tcp_sendmsg iov_len=%lu", iov_len);
        }
    } else {
        size_t count = 0;

        BPF_CORE_READ_INTO(&ubuf, msg, msg_iter.iov, iov_base);
        BPF_CORE_READ_INTO(&count, msg, msg_iter.count);
        log_debug("kprobe/tcp_sendmsg: count=%zu", count);
    }

    log_debug("kprobe/tcp_sendmsg: ubuf=%lx", (unsigned long)ubuf);

    tcp_kprobe_state_t state = {
        .sock = sk,
        .buffer = ubuf,
    };
    bpf_map_update_with_telemetry(tcp_kprobe_state, &pid_tgid, &state, BPF_ANY);
#endif /* COMPILE_CORE */

    return 0;
}

SEC("kretprobe/tcp_sendmsg")
int BPF_KRETPROBE(kretprobe__tcp_sendmsg, int ret) {
    log_debug("kretprobe/tcp_sendmsg ret=%d", ret);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    tcp_kprobe_state_t *state = bpf_map_lookup_elem(&tcp_kprobe_state, &pid_tgid);
    if (!state) {
        log_debug("kretprobe/tcp_sendmsg no state");
        return 0;
    }

    tcp_kprobe_state_t statecopy = *state;
    bpf_map_delete_elem(&tcp_kprobe_state, &pid_tgid);

    if (ret > 0) {
        u64 data0 = 0;
        u64 data1 = 0;
        int ret0, ret1;
        ret0 = bpf_probe_read_user(&data0, sizeof(data0), statecopy.buffer);
        ret1 = bpf_probe_read_user(&data1, sizeof(data1), statecopy.buffer + sizeof(data1));
        log_debug("sendmsg ret0=%d data0=%llx", ret0, bpf_be64_to_cpu(data0));
        log_debug("sendmsg ret1=%d data1=%llx", ret1, bpf_be64_to_cpu(data1));

        kprobe_protocol_dispatcher_entrypoint(ctx, statecopy.sock, statecopy.buffer, ret, false);
    }

    return 0;
}

SEC("kprobe/generic_splice_sendpage")
int BPF_KPROBE(kprobe__generic_splice_sendpage) {
    log_debug("kprobe/generic_splice_sendpage\n");

    u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 splicing = 1;
    bpf_map_update_elem(&tcp_splicing, &pid_tgid, &splicing, BPF_ANY);

    return 0;
}

SEC("kretprobe/generic_splice_sendpage")
int BPF_KRETPROBE(kretprobe__generic_splice_sendpage) {
    log_debug("kretprobe/generic_splice_sendpage");

    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&tcp_splicing, &pid_tgid);

    return 0;
}

SEC("kprobe/splice_to_socket")
int BPF_KPROBE(kprobe__splice_to_socket) {
    log_debug("kprobe/splice_to_socket\n");

    u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 splicing = 1;
    bpf_map_update_elem(&tcp_splicing, &pid_tgid, &splicing, BPF_ANY);

    return 0;
}

SEC("kretprobe/splice_to_socket")
int BPF_KRETPROBE(kretprobe__splice_to_socket) {
    log_debug("kretprobe/splice_to_socket");

    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&tcp_splicing, &pid_tgid);

    return 0;
}

// GO TLS PROBES

// func (c *Conn) Write(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Write")
int BPF_BYPASSABLE_UPROBE(uprobe__crypto_tls_Conn_Write) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = GET_USER_MODE_PID(pid_tgid);
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-write] no offsets data in map for pid %llu", pid);
        return 0;
    }

    // Read the PID and goroutine ID to make the partial call key
    go_tls_function_args_key_t call_key = {0};
    call_key.pid = pid;
    if (read_goroutine_id(ctx, &od->goroutine_id, &call_key.goroutine_id)) {
        log_debug("[go-tls-write] failed reading go routine id for pid %llu", pid);
        return 0;
    }

    // Read the parameters to make the partial call data
    // (since the parameters might not be live by the time the return probe is hit).
    go_tls_write_args_data_t call_data = {0};
    if (read_location(ctx, &od->write_conn_pointer, sizeof(call_data.conn_pointer), &call_data.conn_pointer)) {
        log_debug("[go-tls-write] failed reading conn pointer for pid %llu", pid);
        return 0;
    }

    if (read_location(ctx, &od->write_buffer.ptr, sizeof(call_data.b_data), &call_data.b_data)) {
        log_debug("[go-tls-write] failed reading buffer pointer for pid %llu", pid);
        return 0;
    }

    if (read_location(ctx, &od->write_buffer.len, sizeof(call_data.b_len), &call_data.b_len)) {
        log_debug("[go-tls-write] failed reading buffer length for pid %llu", pid);
        return 0;
    }

    bpf_map_update_elem(&go_tls_write_args, &call_key, &call_data, BPF_ANY);
    return 0;
}

// func (c *Conn) Write(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Write/return")
int BPF_BYPASSABLE_UPROBE(uprobe__crypto_tls_Conn_Write__return) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = GET_USER_MODE_PID(pid_tgid);
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-write-return] no offsets data in map for pid %llu", pid);
        return 0;
    }

    // Read the PID and goroutine ID to make the partial call key
    go_tls_function_args_key_t call_key = {0};
    call_key.pid = pid;

    if (read_goroutine_id(ctx, &od->goroutine_id, &call_key.goroutine_id)) {
        log_debug("[go-tls-write-return] failed reading go routine id for pid %llu", pid);
        return 0;
    }

    uint64_t bytes_written = 0;
    if (read_location(ctx, &od->write_return_bytes, sizeof(bytes_written), &bytes_written)) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] failed reading write return bytes location for pid %llu", pid);
        return 0;
    }

    if (bytes_written <= 0) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] write returned non-positive for amount of bytes written for pid: %llu", pid);
        return 0;
    }

    uint64_t err_ptr = 0;
    if (read_location(ctx, &od->write_return_error, sizeof(err_ptr), &err_ptr)) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] failed reading write return error location for pid %llu", pid);
        return 0;
    }

    // check if err != nil
    if (err_ptr != 0) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] error in write for pid %llu: data will be ignored", pid);
        return 0;
    }

    go_tls_write_args_data_t *call_data_ptr = bpf_map_lookup_elem(&go_tls_write_args, &call_key);
    if (call_data_ptr == NULL) {
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        log_debug("[go-tls-write-return] no write information in write-return for pid %llu", pid);
        return 0;
    }

    conn_tuple_t *t = conn_tup_from_tls_conn(od, (void*)call_data_ptr->conn_pointer, pid_tgid);
    if (t == NULL) {
        log_debug("[go-tls-write-return] failed getting conn tup from tls conn for pid %llu", pid);
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
        return 0;
    }

    char *buffer_ptr = (char*)call_data_ptr->b_data;
    log_debug("[go-tls-write] processing %s", buffer_ptr);
    bpf_map_delete_elem(&go_tls_write_args, &call_key);
    conn_tuple_t copy = {0};
    bpf_memcpy(&copy, t, sizeof(conn_tuple_t));
    // We want to guarantee write-TLS hooks generates the same connection tuple, while read-TLS hooks generate
    // the inverse direction, thus we're normalizing the tuples into a client <-> server direction, and then flipping it
    // to the server <-> client direction.
    normalize_tuple(&copy);
    flip_tuple(&copy);
    tls_process(ctx, &copy, buffer_ptr, bytes_written, GO);
    return 0;
}

// func (c *Conn) Read(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Read")
int BPF_BYPASSABLE_UPROBE(uprobe__crypto_tls_Conn_Read) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = GET_USER_MODE_PID(pid_tgid);
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-read] no offsets data in map for pid %llu", pid);
        return 0;
    }

    // Read the PID and goroutine ID to make the partial call key
    go_tls_function_args_key_t call_key = {0};
    call_key.pid = pid;
    if (read_goroutine_id(ctx, &od->goroutine_id, &call_key.goroutine_id)) {
        log_debug("[go-tls-read] failed reading go routine id for pid %llu", pid);
        return 0;
    }

    // Read the parameters to make the partial call data
    // (since the parameters might not be live by the time the return probe is hit).
    go_tls_read_args_data_t call_data = {0};
    if (read_location(ctx, &od->read_conn_pointer, sizeof(call_data.conn_pointer), &call_data.conn_pointer)) {
        log_debug("[go-tls-read] failed reading conn pointer for pid %llu", pid);
        return 0;
    }
    if (read_location(ctx, &od->read_buffer.ptr, sizeof(call_data.b_data), &call_data.b_data)) {
        log_debug("[go-tls-read] failed reading buffer pointer for pid %llu", pid);
        return 0;
    }

    bpf_map_update_elem(&go_tls_read_args, &call_key, &call_data, BPF_ANY);
    return 0;
}

// func (c *Conn) Read(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Read/return")
int BPF_BYPASSABLE_UPROBE(uprobe__crypto_tls_Conn_Read__return) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = GET_USER_MODE_PID(pid_tgid);
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-read-return] no offsets data in map for pid %llu", pid);
        return 0;
    }

    // On 4.14 kernels we suffered from a verifier issue, that lost track on `call_key` and failed later when accessing
    // to it. The workaround was to delay its creation, so we're getting the goroutine separately.
    __s64 goroutine_id = 0;
    // Read the PID and goroutine ID to make the partial call key
    if (read_goroutine_id(ctx, &od->goroutine_id, &goroutine_id)) {
        log_debug("[go-tls-read-return] failed reading go routine id for pid %llu", pid);
        return 0;
    }

    go_tls_function_args_key_t call_key = {0};
    call_key.pid = pid;
    call_key.goroutine_id = goroutine_id;

    go_tls_read_args_data_t* call_data_ptr = bpf_map_lookup_elem(&go_tls_read_args, &call_key);
    if (call_data_ptr == NULL) {
        log_debug("[go-tls-read-return] no read information in read-return for pid %llu", pid);
        return 0;
    }

    uint64_t bytes_read = 0;
    if (read_location(ctx, &od->read_return_bytes, sizeof(bytes_read), &bytes_read)) {
        log_debug("[go-tls-read-return] failed reading return bytes location for pid %llu", pid);
        bpf_map_delete_elem(&go_tls_read_args, &call_key);
        return 0;
    }

    // Errors like "EOF" of "unexpected EOF" can be treated as no error by the hooked program.
    // Therefore, if we choose to ignore data if read had returned these errors we may have accuracy issues.
    // For now for success validation we chose to check only the amount of bytes read
    // and make sure it's greater than zero.
    if (bytes_read <= 0) {
        log_debug("[go-tls-read-return] read returned non-positive for amount of bytes read for pid: %llu", pid);
        bpf_map_delete_elem(&go_tls_read_args, &call_key);
        return 0;
    }

    conn_tuple_t* t = conn_tup_from_tls_conn(od, (void*) call_data_ptr->conn_pointer, pid_tgid);
    if (t == NULL) {
        log_debug("[go-tls-read-return] failed getting conn tup from tls conn for pid %llu", pid);
        bpf_map_delete_elem(&go_tls_read_args, &call_key);
        return 0;
    }

    char *buffer_ptr = (char*)call_data_ptr->b_data;
    bpf_map_delete_elem(&go_tls_read_args, (go_tls_function_args_key_t*)&call_key);

    // The read tuple should be flipped (compared to the write tuple).
    // tls_process and the appropriate parsers will flip it back if needed.
    conn_tuple_t copy = {0};
    bpf_memcpy(&copy, t, sizeof(conn_tuple_t));
    // We want to guarantee write-TLS hooks generates the same connection tuple, while read-TLS hooks generate
    // the inverse direction, thus we're normalizing the tuples into a client <-> server direction.
    normalize_tuple(&copy);
    tls_process(ctx, &copy, buffer_ptr, bytes_read, GO);
    return 0;
}

// func (c *Conn) Close(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Close")
int BPF_BYPASSABLE_UPROBE(uprobe__crypto_tls_Conn_Close) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    tls_offsets_data_t* od = get_offsets_data();
    if (od == NULL) {
        log_debug("[go-tls-close] no offsets data in map for pid %llu", GET_USER_MODE_PID(pid_tgid));
        return 0;
    }

    // Read the PID and goroutine ID to make the partial call key
    go_tls_function_args_key_t call_key = {0};
    call_key.pid = GET_USER_MODE_PID(pid_tgid);
    if (read_goroutine_id(ctx, &od->goroutine_id, &call_key.goroutine_id) == 0) {
        bpf_map_delete_elem(&go_tls_read_args, &call_key);
        bpf_map_delete_elem(&go_tls_write_args, &call_key);
    }

    void* conn_pointer = NULL;
    if (read_location(ctx, &od->close_conn_pointer, sizeof(conn_pointer), &conn_pointer)) {
        log_debug("[go-tls-close] failed reading close conn pointer for pid %llu", GET_USER_MODE_PID(pid_tgid));
        return 0;
    }

    conn_tuple_t* t = conn_tup_from_tls_conn(od, conn_pointer, pid_tgid);
    if (t == NULL) {
        log_debug("[go-tls-close] failed getting conn tup from tls conn for pid %llu", GET_USER_MODE_PID(pid_tgid));
        return 0;
    }

    // Clear the element in the map since this connection is closed
    bpf_map_delete_elem(&conn_tup_by_go_tls_conn, &conn_pointer);

    conn_tuple_t copy = *t;
    // tls_finish can launch a tail call, thus cleanup should be done before.
    tls_finish(ctx, &copy, false);
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
