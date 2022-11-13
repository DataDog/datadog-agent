#include "kconfig.h"
#include "tracer.h"
#include "bpf_telemetry.h"
#include "ip.h"
#include "ipv6.h"
//#include "http.h"
//#include "https.h"
//#include "http-buffer.h"
#include "sockfd.h"
#include "tags-types.h"
#include "sock.h"
#include "port_range.h"
#include "kafka/socket-filter-approach/kafka-maps.h"
#include "kafka/socket-filter-approach/kafka-types.h"
#include "kafka/socket-filter-approach/kafka-buffer.h"
#include "kafka/socket-filter-approach/kafka.h"

#define SO_SUFFIX_SIZE 3

// This entry point is needed to bypass a memory limit on socket filters
// See: https://datadoghq.atlassian.net/wiki/spaces/NET/pages/2326855913/HTTP#Known-issues
//SEC("socket/http_filter_entry")
SEC("socket/kafka_filter_entry")
int socket__kafka_filter_entry(struct __sk_buff *skb) {
    //bpf_tail_call_compat(skb, &http_progs, HTTP_PROG);
    bpf_tail_call_compat(skb, &kafka_progs, KAFKA_PROG);
    return 0;
}

//SEC("socket/http_filter")
SEC("socket/kafka_filter")
//int socket__http_filter(struct __sk_buff* skb) {
int socket__kafka_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;
    kafka_transaction_t kafka;
    bpf_memset(&kafka, 0, sizeof(kafka));

    if (!read_conn_tuple_skb(skb, &skb_info, &kafka.tup)) {
        return 0;
    }

// Solve the max stack problem
//    if (!kafka_allow_packet(&kafka, skb, &skb_info)) {
//        return 0;
//    }

//    // src_port represents the source port number *before* normalization
//    // for more context please refer to http-types.h comment on `owned_by_src_port` field
//    http.owned_by_src_port = http.tup.sport;
//    normalize_tuple(&http.tup);
    kafka.owned_by_src_port = kafka.tup.sport;
    normalize_tuple(&kafka.tup);
//
//    read_into_buffer_skb((char *)http.request_fragment, skb, &skb_info);
    read_into_buffer_skb((char *)kafka.request_fragment, skb, &skb_info);
//    log_debug("skb->len: %d, info->data_off: %d", skb->len, skb_info.data_off);
//    log_debug("kafka.request_fragment: %d %d %d", kafka.request_fragment[6], kafka.request_fragment[7], kafka.request_fragment[8]);
//    http_process(&http, &skb_info, NO_TAGS);
    kafka_process(&kafka, &skb_info, NO_TAGS);
    return 0;
}
//
//SEC("kprobe/tcp_sendmsg")
//int kprobe__tcp_sendmsg(struct pt_regs* ctx) {
//    log_debug("kprobe/tcp_sendmsg: sk=%llx\n", PT_REGS_PARM1(ctx));
//    // map connection tuple during SSL_do_handshake(ctx)
//    init_ssl_sock_from_do_handshake((struct sock*)PT_REGS_PARM1(ctx));
//    return 0;
//}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb(struct pt_regs* ctx) {
//    log_debug("tracepoint/net/netif_receive_skb\n");
    // flush batch to userspace
    // because perf events can't be sent from socket filter programs
    kafka_flush_batch(ctx);

//    kafka_events *event = bpf_map_lookup_elem(&kafka_events, &key);
//    if (batch == NULL) {
//        return;
//    }
//    bpf_perf_event_output(ctx, &kafka_events, key.cpu, batch, sizeof(kafka_batch_t));
//    log_debug("kafka batch flushed: cpu: %d idx: %d\n", key.cpu, batch->idx);
    return 0;
}

//
//static __always_inline int do_sys_open_helper_enter(struct pt_regs* ctx) {
//    char *path_argument = (char *)PT_REGS_PARM2(ctx);
//    lib_path_t path = {0};
//    if (bpf_probe_read_user_with_telemetry(path.buf, sizeof(path.buf), path_argument) >= 0) {
//// Find the null character and clean up the garbage following it
//#pragma unroll
//        for (int i = 0; i < LIB_PATH_MAX_SIZE; i++) {
//            if (path.len) {
//                path.buf[i] = 0;
//            } else if (path.buf[i] == 0) {
//                path.len = i;
//            }
//        }
//    } else {
//        fill_path_safe(&path, path_argument);
//    }
//
//    // Bail out if the path size is larger than our buffer
//    if (!path.len) {
//        return 0;
//    }
//
//    u64 pid_tgid = bpf_get_current_pid_tgid();
//    path.pid = pid_tgid >> 32;
//    bpf_map_update_with_telemetry(open_at_args, &pid_tgid, &path, BPF_ANY);
//    return 0;
//}
//
//SEC("kprobe/do_sys_open")
//int kprobe__do_sys_open(struct pt_regs* ctx) {
//    return do_sys_open_helper_enter(ctx);
//}
//
//SEC("kprobe/do_sys_openat2")
//int kprobe__do_sys_openat2(struct pt_regs* ctx) {
//    return do_sys_open_helper_enter(ctx);
//}
//
//static __always_inline int do_sys_open_helper_exit(struct pt_regs* ctx) {
//    u64 pid_tgid = bpf_get_current_pid_tgid();
//
//    // If file couldn't be opened, bail out
//    if ((long)PT_REGS_RC(ctx) < 0) {
//        goto cleanup;
//    }
//
//    lib_path_t *path = bpf_map_lookup_elem(&open_at_args, &pid_tgid);
//    if (path == NULL) {
//        return 0;
//    }
//
//    // Detect whether the file being opened is a shared library
//    bool is_shared_library = false;
//#pragma unroll
//    for (int i = 0; i < LIB_PATH_MAX_SIZE - SO_SUFFIX_SIZE; i++) {
//        if (path->buf[i] == '.' && path->buf[i+1] == 's' && path->buf[i+2] == 'o') {
//            is_shared_library = true;
//            break;
//        }
//    }
//
//    if (!is_shared_library) {
//        goto cleanup;
//    }
//
//    // Copy map value into eBPF stack
//    lib_path_t lib_path;
//    __builtin_memcpy(&lib_path, path, sizeof(lib_path));
//
//    u32 cpu = bpf_get_smp_processor_id();
//    bpf_perf_event_output(ctx, &shared_libraries, cpu, &lib_path, sizeof(lib_path));
//cleanup:
//    bpf_map_delete_elem(&open_at_args, &pid_tgid);
//    return 0;
//}
//
//SEC("kretprobe/do_sys_open")
//int kretprobe__do_sys_open(struct pt_regs* ctx) {
//    return do_sys_open_helper_exit(ctx);
//}
//
//SEC("kretprobe/do_sys_openat2")
//int kretprobe__do_sys_openat2(struct pt_regs* ctx) {
//    return do_sys_open_helper_exit(ctx);
//}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
