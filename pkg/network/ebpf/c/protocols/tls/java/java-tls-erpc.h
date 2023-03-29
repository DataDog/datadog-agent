#ifndef _JAVA_TLS_ERPC_H
#define _JAVA_TLS_ERPC_H

#include "bpf_helpers.h"
#include "tracer.h"
#include "../tags-types.h"
#include "../https.h"
#include "port_range.h"
#include "java-tls-types.h"
#include "maps.h"

#define USM_IOCTL_ID 0xda7ad09

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

static void __always_inline handle_hostname(conn_tuple_t* connection, void *data) {

    peer_domain_port_t peer_domain;
    bpf_memset(&peer_domain, 0, sizeof(peer_domain_port_t));
    u64 pid_tgid = bpf_get_current_pid_tgid();
    peer_domain.pid = pid_tgid >> 32;
    peer_domain.port = connection->dport;


    //read the actual domain
    if (0 != bpf_probe_read_user(&peer_domain.domain_name, MAX_DOMAIN_NAME_LENGTH, data)){
        log_debug("[java-tls-handle_hostname] failed reading hostname location for pid %d\n", peer_domain.pid);
        return;
    }

    // register the connection in domain_to_conn_tuple map
    bpf_map_update_with_telemetry(conn_tuple_by_java_peer, &peer_domain, connection, BPF_ANY);

    log_debug("[java-tls-handle_hostname] created map entry for pid %d hostname %s port: %d\n", peer_domain.pid, peer_domain.domain_name, peer_domain.port);
}

static int __always_inline handle_plain(struct pt_regs *ctx, conn_tuple_t* connection, void *data) {
    const bool val = true;
    u32 bytes_read = 0;


    log_debug("[java-tls-handle_plain] starting");
    // Get the buffer the hostname will be read into from a per-cpu array map.
    // Meant to avoid hitting the stack size limit of 512 bytes
    const u32 key = 0;
    peer_domain_port_t* peer_domain = bpf_map_lookup_elem(&java_tls_hostname, &key);
    if (peer_domain == NULL) {
        log_debug("[java-tls-handle_plain] could not get peer domain buffer from map");
        return 1;
    }

    bpf_memset(peer_domain, 0, sizeof(peer_domain_port_t));
    u64 pid_tgid = bpf_get_current_pid_tgid();
    peer_domain->pid = pid_tgid >> 32;
    peer_domain->port = connection->dport;

    //read the actual domain
    if (0 != bpf_probe_read_user(&peer_domain->domain_name, MAX_DOMAIN_NAME_LENGTH, data)){
        log_debug("[java-tls-handle_plain] failed reading hostname location for pid %d\n", peer_domain->pid);
        return 1;
    }

    // get connection tuple
    conn_tuple_t * actual_connection = bpf_map_lookup_elem(&conn_tuple_by_java_peer, peer_domain);
    if (!actual_connection) {
        log_debug("[java-tls-handle_plain] connection not found, pid: %d; hostname: %s; peer port: %d\n",
         peer_domain->pid, peer_domain->domain_name, peer_domain->port);
        return 1;
    }

     log_debug("[java-tls-handle_plain] found correlation conn src port: %d dst port: %d\n",
             actual_connection->sport, actual_connection->dport);

    // read the actual length of the message (limited by HTTP_BUFFER_SIZE)
    if (0 != bpf_probe_read_user(&bytes_read, sizeof(bytes_read), data+MAX_DOMAIN_NAME_LENGTH)){
        log_debug("[java-tls-handle_plain] failed reading message length location for pid %d\n", peer_domain->pid);
        return 1;
    }

    // register the connection in our map
    bpf_map_update_with_telemetry(java_tls_connections, actual_connection, &val, BPF_ANY);
    log_debug("[java-tls-handle_plain] handling tls request of size: %d\n", bytes_read);
    https_process(actual_connection, data+sizeof(bytes_read)+MAX_DOMAIN_NAME_LENGTH, bytes_read, JAVA_TLS);
    http_batch_flush(ctx);
    return 0;
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
    log_debug("[java-tls-handle_erpc_request] received %d op\n", op);
    switch (op) {
    case REQUEST:
        return handle_request(ctx, &connection, data);
    case CLOSE_CONNECTION:
        handle_close_connection(&connection);
        return 0;
    case HOSTNAME:
        handle_hostname(&connection, data);
        return 0;
    case PLAIN:
        return handle_plain(ctx, &connection, data);
    default:
        log_debug("[java-tls-handle_erpc_request] got unsupported erpc request %x for: pid %d\n",op, pid);
    }

    return 0;
}

#endif
