#pragma once

//#include <bpf/bpf_core_read.h>
//#include <bpf/bpf_helpers.h>
//#include <bpf/bpf_tracing.h>
//#include <bpf/bpf_endian.h>

#include "maps.h"
#include "defs.h"

static __always_inline uint32_t as_uint32_t(unsigned char input) {
    return (uint32_t)input;
}

static __always_inline void init_conn_info(uint32_t tgid, int32_t fd, struct conn_info_t* conn_info) {
    conn_info->conn_id.tgid = tgid;
    conn_info->conn_id.fd = fd;
    conn_info->conn_id.tsid = bpf_ktime_get_ns();
    conn_info->role = kRoleUnknown;
    conn_info->current_payload_status.payload_id = 0;
    conn_info->current_payload_status.payload_size = 0;
    conn_info->current_payload_status.is_payload_malformed = false;
}

static __always_inline uint64_t gen_tgid_fd(uint32_t tgid, int fd) {
  return ((uint64_t)tgid << 32) | (uint32_t)fd;
}

static __always_inline bool should_trace_conn(struct conn_info_t* conn_info) {
    struct sock_metadata_t *sock_meta = &conn_info->sock_meta;
    switch (sock_meta->family) {
        case AF_INET:
        case AF_INET6:
            return true;
        default:
            return false;
    }
}

static __always_inline void populate_sock_metadata(const struct socket *socket, struct conn_info_t *conn_info);

static __always_inline void submit_new_conn(void *ctx, uint32_t tgid, int32_t fd,
    const struct sockaddr* addr, const struct socket* socket, enum endpoint_role_t role) {

    struct conn_info_t conn_info = {};
    init_conn_info(tgid, fd, &conn_info);
    if (socket != NULL) {
        populate_sock_metadata(socket, &conn_info);
    }

    conn_info.role = role;
    if (!should_trace_conn(&conn_info)) {
        return;
    }

    uint64_t tgid_fd = gen_tgid_fd(tgid, fd);
    bpf_map_update_elem(&conn_info_map, &tgid_fd, &conn_info, BPF_ANY);
}

static __always_inline enum target_tgid_match_result_t match_trace_tgid(const uint32_t tgid) {
    int idx = kTargetTGIDIndex;
    int64_t* target_tgid = bpf_map_lookup_elem(&control_values, &idx);
    if (target_tgid == NULL) {
        return TARGET_TGID_UNSPECIFIED;
    }
    if (*target_tgid < 0) {
        return TARGET_TGID_ALL;
    }
    if (*target_tgid == tgid) {
        return TARGET_TGID_MATCHED;
    }
    return TARGET_TGID_UNMATCHED;
}

static __always_inline bool is_seekret_tgid(const uint32_t tgid) {
    int idx = kSeekretTGIDIndex;
    int64_t* seekret_tgid = bpf_map_lookup_elem(&control_values, &idx);
    return seekret_tgid != NULL && *seekret_tgid == tgid;
}

static __always_inline void process_syscall_connect(void *ctx, uint64_t id, int ret_val, const struct connect_args_t* args) {
    if (args->fd < 0) {
        return;
    }

    if (ret_val < 0 && ret_val != -EINPROGRESS) {
        return;
    }

    uint32_t tgid = id >> 32;
//    if (match_trace_tgid(tgid) == TARGET_TGID_UNMATCHED) {
//        return;
//    }
//
//    if (is_seekret_tgid(tgid)) {
//        return;
//    }

    submit_new_conn(ctx, tgid, args->fd, args->addr, args->sock_lookup_socket, kRoleClient);
}

static __always_inline void process_syscall_accept(void *ctx, uint64_t id, int ret_fd, const struct accept_args_t* args) {
    if (ret_fd < 0) {
        return;
    }

    uint32_t tgid = id >> 32;
//    if (match_trace_tgid(tgid) == TARGET_TGID_UNMATCHED) {
//        return;
//    }
//
//    if (is_seekret_tgid(tgid)) {
//        return;
//    }

    submit_new_conn(ctx, tgid, ret_fd,args->addr, args->sock_alloc_socket, kRoleServer);
}

static __always_inline bool should_trace_protocol_data(const struct conn_info_t* conn_info) {
    if (conn_info->protocol == kProtocolUnknown) {
        return false;
    }

    uint32_t protocol = conn_info->protocol;
    uint64_t *control = bpf_map_lookup_elem(&control_map, &protocol);
    return control != NULL && *control & conn_info->role;
}

static __always_inline bool should_send_data(struct conn_info_t* conn_info, ssize_t bytes_count) {
    if (conn_info->current_payload_status.is_payload_malformed || conn_info->current_payload_status.payload_size + bytes_count > MAX_PAYLOAD_SIZE_BYTES) {
        return false;
    }

    return should_trace_protocol_data(conn_info);
}

//static __always_inline enum message_type_t infer_http_message(const char* buf, size_t count) {
//    if (count < 16) {
//        return kUnknown;
//    }
//
//    if (buf == NULL) {
//        return kUnknown;
//    }
//
//    char localBuf[7] = {0};
//    bpf_probe_read(localBuf, 7, buf);
//    // HTTP response
//    if (localBuf[0] == 'H' && localBuf[1] == 'T' && localBuf[2] == 'T' && localBuf[3] == 'P') {
//        return kResponse;
//    }
//
//    // Get request
//    if (localBuf[0] == 'G' && localBuf[1] == 'E' && localBuf[2] == 'T') {
//        return kRequest;
//    }
//
//    // Head request
//    if (localBuf[0] == 'H' && localBuf[1] == 'E' && localBuf[2] == 'A' && localBuf[3] == 'D') {
//        return kRequest;
//    }
//
//    // Post request
//    if (localBuf[0] == 'P' && localBuf[1] == 'O' && localBuf[2] == 'S' && localBuf[3] == 'T') {
//        return kRequest;
//    }
//
//    // Put request
//    if (localBuf[0] == 'P' && localBuf[1] == 'U' && localBuf[2] == 'T') {
//        return kRequest;
//    }
//
//    // Delete request
//    if (localBuf[0] == 'D' && localBuf[1] == 'E' && localBuf[2] == 'L' && localBuf[3] == 'E' && localBuf[4] == 'T' && localBuf[5] == 'E') {
//        return kRequest;
//    }
//
//    // Patch
//    if (localBuf[0] == 'P' && localBuf[1] == 'A' && localBuf[2] == 'T' && localBuf[3] == 'C' && localBuf[4] == 'H') {
//        return kRequest;
//    }
//
//    // Trace
//    if (localBuf[0] == 'T' && localBuf[1] == 'R' && localBuf[2] == 'A' && localBuf[3] == 'C' && localBuf[4] == 'E') {
//        return kRequest;
//    }
//
//    // Options
//    if (localBuf[0] == 'O' && localBuf[1] == 'P' && localBuf[2] == 'T' && localBuf[3] == 'I' && localBuf[4] == 'O' && localBuf[5] == 'N' && localBuf[6] == 'S') {
//        return kRequest;
//    }
//
//    // Connect
//    if (localBuf[0] == 'C' && localBuf[1] == 'O' && localBuf[2] == 'N' && localBuf[3] == 'N' && localBuf[4] == 'E' && localBuf[5] == 'C' && localBuf[6] == 'T') {
//        return kRequest;
//    }
//    return kUnknown;
//}


//static __always_inline bool starts_with_http2_marker(const char* buf, size_t buf_size) {
//    if (buf_size < HTTP2_MARKER_SIZE) {
//        return false;
//    }
//
//    if (buf == NULL) {
//        return false;
//    }
//
//    uint8_t localBuf[HTTP2_MARKER_SIZE] = {0};
//    bpf_probe_read_user(localBuf, HTTP2_MARKER_SIZE, buf);
//    uint8_t http2_prefix[] = {0x50, 0x52, 0x49, 0x20, 0x2a, 0x20, 0x48, 0x54, 0x54, 0x50, 0x2f, 0x32, 0x2e, 0x30, 0x0d, 0x0a, 0x0d, 0x0a, 0x53, 0x4d, 0x0d, 0x0a, 0x0d, 0x0a};
//    for (int i = 0; i < HTTP2_MARKER_SIZE; i++) {
//        if (localBuf[i] != http2_prefix[i]) {
//            return false;
//        }
//    }
//
//    return true;
//}

//static __always_inline enum message_type_t infer_http2_message(const char* buf, size_t count) {
//    if (!starts_with_http2_marker(buf, count)) {
//        return kUnknown;
//    }
//    return kRequest;
//}

static __inline int32_t read_big_endian_int32(const char* buf) {
  int32_t length;
  bpf_probe_read(&length, 4, (void*)buf);
  return bpf_ntohl(length);
}

typedef signed short int16_t;

static __inline int32_t read_big_endian_int16(const char* buf) {
  int16_t val;
  bpf_probe_read(&val, 2, (void*)buf);
  return bpf_ntohl(val);
}

// Reference: https://kafka.apache.org/protocol.html#protocol_messages
// Request Header v0 => request_api_key request_api_version correlation_id
//     request_api_key => INT16
//     request_api_version => INT16
//     correlation_id => INT32
static __inline enum message_type_t infer_kafka_request(const char* buf) {
    // API is Kafka's terminology for opcode.
    static const int kNumAPIs = 68;
    static const int kMaxAPIVersion = 13;

    const int16_t request_API_key = read_big_endian_int16(buf);
    if (request_API_key < 0 || request_API_key > kNumAPIs) {
        return kUnknown;
    }

    const int16_t request_API_version = read_big_endian_int16(buf + 2);
    if (request_API_version < 0 || request_API_version > kMaxAPIVersion) {
        return kUnknown;
    }

    const int32_t correlation_id = read_big_endian_int32(buf + 4);
    if (correlation_id < 0) {
        return kUnknown;
    }

    return kRequest;
}


static __always_inline enum message_type_t infer_kafka_message(const char *buf, size_t count) {
    static const int kMinRequestLength = 12;
    if (count < kMinRequestLength) {
        return kUnknown;
    }

    const int32_t message_size = read_big_endian_int32(buf) + 4;

    // Enforcing count to be exactly message_size + 4 to mitigate mis-classification.
    // However, this will miss long messages broken into multiple reads.
    if (message_size < 0 || count != (size_t)message_size) {
        return kUnknown;
    }

    return infer_kafka_request(buf + 4);
}

static __always_inline struct protocol_message_t infer_protocol(const char* buf, size_t count, struct conn_info_t* conn_info) {
    struct protocol_message_t inferred_message;
    inferred_message.protocol = kProtocolUnknown;
    inferred_message.type = kUnknown;

    if ((inferred_message.type = infer_kafka_message(buf, count)) != kUnknown) {
        inferred_message.protocol = kProtocolKafka;
//    } else if ((inferred_message.type = infer_http2_message(buf, count)) != kUnknown) {
//        inferred_message.protocol = kProtocolHTTP2;
//    } else if ((inferred_message.type = infer_http_message(buf, count)) != kUnknown) {
//        inferred_message.protocol = kProtocolHTTP;
    }

    return inferred_message;
}

static __always_inline void update_traffic_class(struct conn_info_t* conn_info, enum traffic_direction_t direction,
    const char* buf, size_t count) {
    if (conn_info == NULL) {
        return;
    }
    struct protocol_message_t inferred_protocol = infer_protocol(buf, count, conn_info);

    if (inferred_protocol.protocol == kProtocolUnknown) {
        return;
    }

    if (inferred_protocol.type == kRequest) {
        conn_info->current_payload_status.payload_id = bpf_ktime_get_ns();
        conn_info->current_payload_status.payload_size = 0;
        conn_info->current_payload_status.is_payload_malformed = false;
    }

    if (conn_info->protocol == kProtocolUnknown) {
        conn_info->protocol = inferred_protocol.protocol;
    }

    if (conn_info->role == kRoleUnknown && inferred_protocol.type != kUnknown) {
        conn_info->role = ((direction == kEgress) ^ (inferred_protocol.type == kResponse))
                              ? kRoleClient
                              : kRoleServer;
    }
}

static __always_inline struct socket_data_event_t* fill_socket_data_event(enum traffic_direction_t direction, const struct conn_info_t* conn_info) {
    uint32_t kZero = 0;
    struct socket_data_event_t* event = bpf_map_lookup_elem(&socket_data_event_buffer_heap, &kZero);
    if (event == NULL) {
        return NULL;
    }
    event->attr.timestamp_ns = bpf_ktime_get_ns();
    event->attr.direction = direction;
    event->attr.conn_id = conn_info->conn_id;
    event->attr.protocol = conn_info->protocol;
    event->attr.role = conn_info->role;
    event->attr.payload_id = conn_info->current_payload_status.payload_id;
    event->attr.sock_metadata = conn_info->sock_meta;
    return event;
}

static __always_inline void perf_submit_buf(void *ctx, enum traffic_direction_t direction,
    const char* buf, size_t buf_size, size_t offset, struct conn_info_t* conn_info, struct socket_data_event_t* event) {
    switch (direction) {
        case kEgress:
            event->attr.pos = conn_info->wr_bytes + offset;
            break;
        case kIngress:
            event->attr.pos = conn_info->rd_bytes + offset;
            break;
    }

    if (buf_size == 0) {
        return;
    }

    size_t buf_size_minus_1 = buf_size - 1;
    // taken from pixie, prevents from clang to optimize the code here and thus the verifier does not go crazy on us.
    asm volatile("" : "+r"(buf_size_minus_1) :);
    buf_size = buf_size_minus_1 + 1;

    const u64 buf_size_to_override_verifier = buf_size < (MAX_EVENT_DATA_SIZE + 1) ? buf_size : MAX_EVENT_DATA_SIZE;
    bpf_probe_read_user(event->msg, buf_size_to_override_verifier, (void*)buf);
    event->attr.msg_size = (uint32_t)buf_size_to_override_verifier;

    const u64 total_event_size = sizeof(event->attr) + event->attr.msg_size;
    if (total_event_size <= MAX_EVENT_DATA_SIZE && total_event_size > 0) {
        bpf_perf_event_output(ctx, &socket_data_events, BPF_F_CURRENT_CPU, event, total_event_size);
    }
}

static __always_inline void perf_submit_wrapper(void *ctx, enum traffic_direction_t direction, const char* buf,
    const size_t buf_size, struct conn_info_t* conn_info, struct socket_data_event_t* event) {
    if (buf_size > MAX_PAYLOAD_SIZE_BYTES) {
        return;
    }
    int bytes_sent = 0;
    unsigned int i;
#pragma unroll
    for (i = 0; i < MAX_ITERATIONS_FOR_DATA_EVENT; ++i) {
        const int bytes_remaining = buf_size - bytes_sent;
        if (bytes_remaining <= 0) {
            break;
        }
        const size_t current_size =  (bytes_remaining > MAX_EVENT_DATA_SIZE && (i != MAX_ITERATIONS_FOR_DATA_EVENT - 1)) ? MAX_EVENT_DATA_SIZE : bytes_remaining;
        perf_submit_buf(ctx, direction, buf + bytes_sent, current_size, bytes_sent, conn_info, event);
        bytes_sent += current_size;
    }
}

static __always_inline void perf_submit_iovecs(void *ctx, enum traffic_direction_t direction,
    const struct iovec* iov, size_t iovlen, size_t total_size, struct conn_info_t* conn_info, struct socket_data_event_t* event) {
    if (total_size > MAX_PAYLOAD_SIZE_BYTES) {
        return;
    }
    int bytes_sent = 0;
#pragma unroll
    for (unsigned int i = 0; i < 42 && i < iovlen && bytes_sent < total_size; ++i) {
        struct iovec iov_cpy;
        bpf_probe_read(&iov_cpy, sizeof(struct iovec), (void*)&iov[i]);
        const int bytes_remaining = total_size - bytes_sent;
        const size_t iov_size = iov_cpy.iov_len < bytes_remaining ? iov_cpy.iov_len : bytes_remaining;
        perf_submit_buf(ctx, direction, iov_cpy.iov_base, iov_size, bytes_sent, conn_info, event);
        bytes_sent += iov_size;
    }
}

static __always_inline void update_conn_stats(void *ctx, struct conn_info_t* conn_info,
    enum traffic_direction_t direction, ssize_t bytes_count) {
    conn_info->current_payload_status.payload_size += bytes_count;
    switch (direction) {
    case kEgress:
        conn_info->wr_bytes += bytes_count;
        break;
    case kIngress:
        conn_info->rd_bytes += bytes_count;
        break;
    }
}

static __always_inline void submit_malformed_event(void *ctx, struct conn_info_t* conn_info) {
    struct malformed_socket_event_t event = {};
    event.close_timestamp_ns = bpf_ktime_get_ns();
    event.conn_id = conn_info->conn_id;
    event.payload_id = conn_info->current_payload_status.payload_id;

    conn_info->current_payload_status.is_payload_malformed = true;

    bpf_perf_event_output(ctx, &malformed_socket_events, BPF_F_CURRENT_CPU, &event, sizeof(struct malformed_socket_event_t));
}

//static __always_inline void perf_submit_http2_frames(void *ctx, const char *buf, size_t bytes_count, enum traffic_direction_t direction, struct conn_info_t *conn_info) {
//    size_t pos = 0;
//#pragma unroll
//    for (uint32_t i = 0; i < HTTP2_MAX_FRAMES; ++i) {
//        struct http2_frame current_frame = {};
//        if (!read_http2_frame_header(buf + pos, bytes_count-pos, &current_frame)) {
//            break;
//        }
//        const size_t current_frame_total_length = HTTP2_FRAME_HEADER_SIZE + current_frame.length;
//        if (current_frame.type != kDataFrame && current_frame.type != kHeadersFrame) {
//            pos += current_frame_total_length;
//            continue;
//        }
//        if (pos + current_frame_total_length > bytes_count) {
//            break;
//        }
//
//        struct socket_data_event_t* event = fill_socket_data_event(direction, conn_info);
//        if (event == NULL) {
//            break;
//        }
//
//        perf_submit_buf(ctx, direction, buf + pos, current_frame_total_length, pos, conn_info, event);
//        update_conn_stats(ctx, conn_info, direction, current_frame_total_length);
//        pos += current_frame_total_length;
//        if (pos >= bytes_count) {
//            break;
//        }
//    }
//}

static __always_inline void process_data(void *ctx, uint64_t id, enum traffic_direction_t direction,
    const struct data_args_t* args, ssize_t bytes_count, bool is_tls) {
    if (args->fd < 0) {
        return;
    }

    if (bytes_count <= 0) {
        return;
    }

    uint32_t tgid = id >> 32;
    uint64_t tgid_fd = gen_tgid_fd(tgid, args->fd);
    struct conn_info_t* conn_info = bpf_map_lookup_elem(&conn_info_map, &tgid_fd);
    if (conn_info == NULL) {
        return;
    }

//    if (is_tls != conn_info->is_tls) {
//        // Wasn't called from SSL context.
//        return;
//    }

    update_traffic_class(conn_info, direction, args->buf, bytes_count);
//    if (conn_info->protocol == kProtocolHTTP2) {
//        if (!should_trace_protocol_data(conn_info)) {
//            return;
//        }
//        // HTTP2 uniqueness requires special treatment.
//        // 1. We must strip the marker from the connection
//        // 2. We must parse the connection and send frame by frame.
//        // 3. We cannot send "malformed event".
//        char *buf = (char*)args->buf;
//        if (starts_with_http2_marker(buf, bytes_count)) {
//            buf += HTTP2_MARKER_SIZE;
//            bytes_count -= HTTP2_MARKER_SIZE;
//        }
//
//        perf_submit_http2_frames(ctx, buf, bytes_count, direction, conn_info);
//        return;
//    }

    if (should_send_data(conn_info, bytes_count)) {
        struct socket_data_event_t* event = fill_socket_data_event(direction, conn_info);
        if (event == NULL) {
            return;
        }

        perf_submit_wrapper(ctx, direction, args->buf, bytes_count, conn_info, event);
        update_conn_stats(ctx, conn_info, direction, bytes_count);
    } else {
        submit_malformed_event(ctx, conn_info);
    }
}

static __always_inline void process_plaintext_data(void *ctx, uint64_t id, enum traffic_direction_t direction,
    const struct data_args_t* args, ssize_t bytes_count) {
    process_data(ctx, id, direction, args, bytes_count, false);
}

//static __always_inline void process_tls_data(void *ctx, uint64_t id, enum traffic_direction_t direction,
//    const struct data_args_t* args, ssize_t bytes_count) {
//    process_data(ctx, id, direction, args, bytes_count, true);
//}

static __always_inline void process_syscall_data_vecs(void *ctx, uint64_t id, enum traffic_direction_t direction,
    const struct data_args_t* args, ssize_t bytes_count) {

    if (args->fd < 0 || bytes_count <= 0) {
        return;
    }

    if (args->iov == NULL || args->iovlen <= 0) {
        return;
    }

    uint32_t tgid = id >> 32;
    uint64_t tgid_fd = gen_tgid_fd(tgid, args->fd);
    struct conn_info_t* conn_info = bpf_map_lookup_elem(&conn_info_map, &tgid_fd);
    if (conn_info == NULL) {
        return;
    }

    struct iovec io_vec;
    bpf_probe_read(&io_vec, sizeof(struct iovec), (void*)&args->iov[0]);
    size_t buf_size = bytes_count;
    if (io_vec.iov_len < bytes_count) {
        buf_size = io_vec.iov_len;
    }

    update_traffic_class(conn_info, direction, io_vec.iov_base, buf_size);
    if (should_send_data(conn_info, bytes_count)) {
        struct socket_data_event_t* event = fill_socket_data_event(direction, conn_info);
        if (event == NULL) {
            return;
        }
        perf_submit_iovecs(ctx, direction, args->iov, args->iovlen, bytes_count, conn_info, event);
        update_conn_stats(ctx, conn_info, direction, bytes_count);
    } else {
        submit_malformed_event(ctx, conn_info);
    }
}

static __always_inline void process_implicit_conn(void *ctx, uint64_t id, const struct connect_args_t* args) {
    if (args->fd < 0) {
        return;
    }

    uint32_t tgid = id >> 32;
    uint64_t tgid_fd = gen_tgid_fd(tgid, args->fd);
    if (match_trace_tgid(tgid) == TARGET_TGID_UNMATCHED) {
        return;
    }
    if (is_seekret_tgid(tgid)) {
        return;
    }

    struct conn_info_t* conn_info = bpf_map_lookup_elem(&conn_info_map, &tgid_fd);
    if (conn_info != NULL) {
        return;
    }
    submit_new_conn(ctx, tgid, args->fd, args->addr, NULL, kRoleUnknown);
}

static __always_inline void submit_close_event(void *ctx, struct conn_info_t* conn_info) {
    struct socket_close_event_t event = {};
    event.close_timestamp_ns = bpf_ktime_get_ns();
    event.conn_id = conn_info->conn_id;
    event.total_rd_bytes = conn_info->rd_bytes;
    event.total_wr_bytes = conn_info->wr_bytes;
    event.role = conn_info->role;

    bpf_perf_event_output(ctx, &socket_close_events, BPF_F_CURRENT_CPU, &event, sizeof(struct socket_close_event_t));
}

static __always_inline void process_syscall_close(void *ctx, uint64_t id, int ret_val, const struct close_args_t* close_args) {
    uint32_t tgid = id >> 32;

    if (close_args->fd < 0) {
        return;
    }
    if (ret_val < 0) {
        return;
    }

    uint64_t tgid_fd = gen_tgid_fd(tgid, close_args->fd);
    struct conn_info_t* conn_info = bpf_map_lookup_elem(&conn_info_map, &tgid_fd);
    if (conn_info == NULL) {
        return;
    }
    bpf_map_delete_elem(&conn_info_map, &tgid_fd);

    submit_close_event(ctx, conn_info);
}

//static __always_inline void process_syscall_bind(struct trace_event_raw_sys_exit *ctx, uint64_t id, int ret_val, const struct bind_args_t* bind_args) {
//    if (ret_val < 0) {
//            return;
//    }
//
//    const struct sockaddr* socket_address = bind_args->addr;
//    switch (BPF_CORE_READ_USER(socket_address, sa_family)) {
//        case AF_INET:
//            // Handle IPv4
//            break;
//        case AF_INET6:
//            // Handle IPv6
//            break;
//        default:
//            return;
//    }
//
//    uint32_t tgid = (id >> 32);
//    bpf_perf_event_output(ctx, &bind_pid_events, BPF_F_CURRENT_CPU, &tgid, sizeof(tgid));
//}

static __always_inline struct sock* get_socket_sock(const struct socket *socket) {
    //return BPF_CORE_READ(socket, sk);
    struct sock* return_value = 0;
    bpf_probe_read(&return_value, sizeof(return_value), (void*)&socket->sk);
    return return_value;
}

static __always_inline u16 get_sock_family(const struct sock *sock) {
    //return BPF_CORE_READ(sock, __sk_common.skc_family);
    u16 return_value = 0;
    bpf_probe_read(&return_value, sizeof(return_value), (void*)&(sock->__sk_common.skc_family));
    return return_value;

}

static __always_inline u32 get_inet_saddr(const struct inet_sock *inet) {
    //return BPF_CORE_READ(inet, inet_saddr);
    u32 return_value = 0;
    bpf_probe_read(&return_value, sizeof(return_value), (void*)&(inet->inet_saddr));
    return return_value;
}

static __always_inline u32 get_inet_daddr(const struct inet_sock *inet) {
    //return BPF_CORE_READ(inet, sk.__sk_common.skc_daddr);
    u32 return_value = 0;
    bpf_probe_read(&return_value, sizeof(return_value), (void*)&(inet->sk.__sk_common.skc_daddr));
    return return_value;
}

static __always_inline u16 get_inet_sport(const struct inet_sock *inet) {
    //return BPF_CORE_READ(inet, inet_sport);
    u16 return_value = 0;
    bpf_probe_read(&return_value, sizeof(return_value), (void*)&(inet->inet_sport));
    return return_value;
}

static __always_inline u16 get_inet_dport(const struct inet_sock *inet) {
    //return BPF_CORE_READ(inet, sk.__sk_common.skc_dport);
    u16 return_value = 0;
    bpf_probe_read(&return_value, sizeof(return_value), (void*)&(inet->sk.__sk_common.skc_dport));
    return return_value;
}

static __always_inline struct in6_addr get_sock_v6_rcv_saddr(const struct sock *sock) {
    //return BPF_CORE_READ(sock, __sk_common.skc_v6_rcv_saddr);
    struct in6_addr return_value;
    bpf_probe_read(&return_value, sizeof(return_value), (void*)&(sock->__sk_common.skc_v6_rcv_saddr));
    return return_value;
}

static __always_inline struct in6_addr get_sock_v6_daddr(const struct sock *sock) {
    //return BPF_CORE_READ(sock, __sk_common.skc_v6_daddr);
    struct in6_addr return_value;
    bpf_probe_read(&return_value, sizeof(return_value), (void*)&(sock->__sk_common.skc_v6_daddr));
    return return_value;
}

static __always_inline void get_network_details_from_sock_v4(struct sock *sk, struct sock_metadata_t *sock_metadata) {
    struct inet_sock *inet = (struct inet_sock *)sk;
    sock_metadata->ipv4.saddr = get_inet_saddr(inet);
    sock_metadata->ipv4.daddr = get_inet_daddr(inet);
    sock_metadata->sport = bpf_ntohs(get_inet_sport(inet));
    sock_metadata->dport = bpf_ntohs(get_inet_dport(inet));
}

static __always_inline void get_network_details_from_sock_v6(struct sock *sk, struct sock_metadata_t *sock_metadata) {
    struct inet_sock *inet = (struct inet_sock *)sk;
    sock_metadata->ipv6.saddr = get_sock_v6_rcv_saddr(sk);
    sock_metadata->ipv6.daddr = get_sock_v6_daddr(sk);
    sock_metadata->sport = bpf_ntohs(get_inet_sport(inet));
    sock_metadata->dport = bpf_ntohs(get_inet_dport(inet));
}

static __always_inline void populate_sock_metadata(const struct socket *socket, struct conn_info_t *conn_info) {
    struct sock* sk = get_socket_sock(socket);

    u16 family = get_sock_family(sk);
    if (family > AF_MAX || family == AF_UNSPEC) {
        return;
    }

    conn_info->sock_meta.family = family;
    switch (family) {
    case AF_INET:
        get_network_details_from_sock_v4(sk, &conn_info->sock_meta);
        return;
    case AF_INET6:
        get_network_details_from_sock_v6(sk, &conn_info->sock_meta);
        return;
    }
}

static __always_inline int* get_tls_fd_from_context(uint64_t tls_context_as_number, uint64_t id) {
    struct tls_ctx_to_fd_key_t tls_ctx_to_fd_key = {};
    tls_ctx_to_fd_key.id = id;
    tls_ctx_to_fd_key.tls_context_as_number = tls_context_as_number;
    return bpf_map_lookup_elem(&tls_ctx_to_fd_map, &tls_ctx_to_fd_key);
}
