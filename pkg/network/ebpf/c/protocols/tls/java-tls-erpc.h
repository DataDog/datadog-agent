#ifndef _JAVA_TLS_ERPC_H
#define _JAVA_TLS_ERPC_H

#include "bpf_helpers.h"
#include "tracer.h"
#include "tags-types.h"
#include "https.h"
#include "port_range.h"

#define USM_IOCTL_ID 0xda7ad09

enum erpc_message_type {
    REQUEST,
    CLOSE_CONNECTION
};

/*
  handle_request pseudo format of *data that contain the http payload

  struct {
      u32 len;
      u8 data[len];
  }
*/
static int __always_inline handle_request(struct pt_regs *ctx, conn_tuple_t* connection, void *data) {
    const bool val = true;
    u32 bytes_read = 0;

    // read the actual length of the message (limited by HTTP_BUFFER_SIZE)
    if (0 != bpf_probe_read_user(&bytes_read, sizeof(bytes_read), data)){
#ifdef DEBUG
        u64 pid_tgid = bpf_get_current_pid_tgid();
        u64 pid = pid_tgid >> 32;
        log_debug("[java-tls-handle_request] failed reading message length location for pid %d\n", pid);
#endif
        return 1;
    }
    // register the connection in our map
    bpf_map_update_with_telemetry(java_tls_connections, connection, &val, BPF_ANY);
    log_debug("[java-tls-handle_request] handling tls request of size: %d\n", bytes_read);
    https_process(connection, data+sizeof(bytes_read), bytes_read, JAVA_TLS);
    http_batch_flush(ctx);
    return 0;
}

static void __always_inline handle_close_connection(conn_tuple_t* connection) {
    void *exists = bpf_map_lookup_elem(&java_tls_connections, connection);
    // if the connection exists in our map, finalize it and remove from the map
    // otherwise just ignore
    if (exists != NULL){
        https_finish(connection);
        log_debug("[java-tls-handle_request] removing connection from the map %llx\n", connection->daddr_h);
        bpf_map_delete_elem(&java_tls_connections, connection);
    }
}

static int __always_inline is_usm_erpc_request(struct pt_regs *ctx) {
    u32 cmd = PT_REGS_PARM3(ctx);
    return cmd == USM_IOCTL_ID;
}

/*
  handle_erpc_request ioctl request format :

  struct {
      u8           operation;  // REQUEST, CLOSE_CONNECTION
      conn_tuple_t connection; // connection tuple
      u8           data[];     // payload data
  }
*/
static int __always_inline handle_erpc_request(struct pt_regs *ctx) {
#ifdef DEBUG
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = pid_tgid >> 32;
#endif
    void *req = (void *)PT_REGS_PARM4(ctx);

    u8 op = 0;
    if (0 != bpf_probe_read_user(&op, sizeof(op), req)){
        log_debug("[java-tls-handle_erpc_request] failed to parse opcode of java tls erpc request for: pid %d\n", pid);
        return 1;
    }

    // get connection tuple
    conn_tuple_t connection = {0};
    if (0 != bpf_probe_read_user(&connection, sizeof(conn_tuple_t), req+sizeof(op))){
        log_debug("[java-tls-handle_erpc_request] failed to parse connection info of java tls erpc request %x for: pid %d\n", op, pid);
        return 1;
    }

    normalize_tuple(&connection);

    void *data = req + sizeof(op) + sizeof(conn_tuple_t);
    switch (op) {
    case REQUEST:
        return handle_request(ctx, &connection, data);
    case CLOSE_CONNECTION:
        handle_close_connection(&connection);
        return 0;
    default:
        log_debug("[java-tls-handle_erpc_request] got unsupported erpc request %x for: pid %d\n",op, pid);
    }

    return 0;
}

#endif
