#ifndef _JAVA_TLS_ERPC_H
#define _JAVA_TLS_ERPC_H

#include "bpf_helpers.h"
#include "tracer.h"
#include "tags-types"

#define USM_IOCTL_ID 0xda7ad09

enum erpc_message_type {
    REQUEST,
    CLOSE_CONNECTION
};


int __attribute__((always_inline)) handle_request(conn_tuple_t* connection, void *data,) {
    //read the actual length of the message (limited by HTTP_BUFFER_SIZE)
    u32 bytes_read = 0;
    if (0 != bpf_probe_user_read(&bytes_read, sizeof(bytes_read), data)){
        u64 pid_tgid = bpf_get_current_pid_tgid();
        u64 pid = pid_tgid >> 32;
        log_debug("[java-tls-handle_request] failed reading message length location for pid %d\n", pid);
        return 1;
    }
    https_process(connection, data+sizeof(bytes_read), bytes_read, JAVA_TLS);
    return 0;
}

void __attribute__((always_inline)) handle_close_connection(conn_tuple_t* connection, void *data) {
    void *exists = bpf_map_lookup_elem(&java_tls_connections, &connection);
    // if the connection exists in our map, finalize it and remove from the map
    // otherwise just ignore
    if (exists != NULL){
        https_finish(connection);
        bpf_map_delete_elem(&java_tls_connections,&connection);
    }
}

int __attribute__((always_inline)) is_usm_erpc_request(struct pt_regs *ctx) {
    u32 cmd = PT_REGS_PARM3(ctx);
    if (cmd != USM_IOCTL_ID) {
        return 0;
    }

    return 1;
}

int __attribute__((always_inline)) handle_erpc_request(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 pid = pid_tgid >> 32;
    void *req = (void *)PT_REGS_PARM4(ctx);

    u8 op = 0;
    if (0 != bpf_probe_user_read(&op, sizeof(op), req)){
        log_debug("[java-tls-handle_erpc_request] failed to parse opcode of java tls erpc request for: pid %d\n", pid);
        return 1;
    }

    //get connection tuple
    conn_tuple_t connection = {0};
    if (0 != bpf_probe_user_read(&connection, sizeof(conn_tuple_t), req+sizeof(op))){
        log_debug("[java-tls-handle_erpc_request] failed to parse connection info of java tls erpc request %x for: pid %d\n",op, pid);
        return 1;
    }

    void *data = req + sizeof(op) + sizeof(conn_tuple_t);
    switch (op) {
        case REQUEST:
            return handle_request(&connection, data);
        case CLOSE_CONNECTION:
            handle_close_connection(&connection, data);
            return 0;
        default:
            log_debug("[java-tls-handle_erpc_request] got unsupported erpc request %x for: pid %d\n",op, pid);

    }

    return 0;
}

#endif
