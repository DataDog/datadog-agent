#ifndef __ERPC_HANDLERS_H
#define __ERPC_HANDLERS_H

#include "conn_tuple.h"
#include "protocols/tls/tags-types.h"
#include "protocols/tls/https.h"
#include "port_range.h"

// macro to get the data pointer from the ctx, we skip the 1st byte as it is the operation byte read by the erpc dispatcher
#define GET_DATA_PTR(ctx) ((void *)(PT_REGS_PARM4(ctx) + 1))

/*
  handle_sync_payload's pseudo format of *data that contains the http payload

  struct {
      conn_tuple_t;
      u32 payload_len;
      u8 payload_buffer[payload_len];
  }
*/

SEC("kprobe/handle_sync_payload")
int kprobe_handle_sync_payload(struct pt_regs *ctx) {
    // get connection tuple
    conn_tuple_t connection = {0};
    const bool val = true;
    u32 bytes_read = 0;

    //interactive pointer to read the data buffer
    void* bufferPtr = GET_DATA_PTR(ctx);

    //read the connection tuple from the ioctl buffer
    if (0 != bpf_probe_read_user(&connection, sizeof(conn_tuple_t), bufferPtr)){
        log_debug("[handle_sync_payload] failed to parse connection info\n");
        return 1;
    }
    normalize_tuple(&connection);
    bufferPtr+=sizeof(conn_tuple_t);

    // read the actual length of the message (limited by HTTP_BUFFER_SIZE)
    if (0 != bpf_probe_read_user(&bytes_read, sizeof(bytes_read), bufferPtr)){
#ifdef DEBUG
        u64 pid_tgid = bpf_get_current_pid_tgid();
        u64 pid = pid_tgid >> 32;
        log_debug("[handle_sync_payload] failed reading message length location for pid %d\n", pid);
#endif
        return 1;
    }
    bufferPtr+=sizeof(bytes_read);

    // register the connection in our map
    bpf_map_update_elem(&java_tls_connections, &connection, &val, BPF_ANY);
    log_debug("[handle_sync_payload] handling tls request of size: %d for connection src addr: %llx; dst address %llx\n",
                bytes_read, connection.saddr_l, connection.daddr_l);
    tls_process(ctx, &connection, bufferPtr, bytes_read, JAVA_TLS);
    return 0;
}

/*
  handle_close_connection gets only the connection information in form of conn_tuple_t struct from the close event of the socket
*/
SEC("kprobe/handle_close_connection")
int kprobe_handle_close_connection(struct pt_regs *ctx) {
    //interactive pointer to read the data buffer
    void* bufferPtr = GET_DATA_PTR(ctx);
    //read the connection tuple from the ioctl buffer
    conn_tuple_t connection = {0};
    if (0 != bpf_probe_read_user(&connection, sizeof(conn_tuple_t), bufferPtr)){
        log_debug("[java_tls_handle_close] failed to parse connection info\n");
        return 1;
    }
    normalize_tuple(&connection);

    void *exists = bpf_map_lookup_elem(&java_tls_connections, &connection);
    // if the connection exists in our map, finalize it and remove from the map
    // otherwise just ignore
    if (exists != NULL){
        // tls_finish can launch a tail call, thus cleanup should be done before.
        bpf_map_delete_elem(&java_tls_connections, &connection);
        tls_finish(ctx, &connection);
    }
    return 0;
}

/*
  handle_connection_by_peer gets connection information along the peer domain and port information
  which helps to correlate later the plain payload with the relevant connection via the peer details
*/
SEC("kprobe/handle_connection_by_peer")
int kprobe_handle_connection_by_peer(struct pt_regs *ctx) {

    connection_by_peer_key_t peer_key ={0};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    peer_key.pid = pid_tgid >> 32;

    //interactive pointer to read the data buffer
    void* bufferPtr = GET_DATA_PTR(ctx);

    //read the connection tuple from the ioctl buffer
    conn_tuple_t connection = {0};
    if (0 != bpf_probe_read_user(&connection, sizeof(conn_tuple_t), bufferPtr)){
        log_debug("[handle_connection_by_peer] failed to parse connection info for pid: %d\n", peer_key.pid);
        return 1;
    }
    normalize_tuple(&connection);
    bufferPtr+=sizeof(conn_tuple_t);

    //read the peer tuple (domain string and port)
    if (0 != bpf_probe_read_user(&peer_key.peer, sizeof(peer_t), bufferPtr)){
        log_debug("[handle_connection_by_peer] failed reading peer tuple information for pid %d\n", peer_key.pid);
        return 1;
    }

    // register the connection in conn_by_peer map
    bpf_map_update_elem(&java_conn_tuple_by_peer, &peer_key, &connection, BPF_ANY);

    log_debug("[handle_connection_by_peer] created map entry for pid %d domain %s port: %d\n",
                peer_key.pid, peer_key.peer.domain, peer_key.peer.port);
    return 0;
}

/*
  handle_async_payload doesn't contain any transport layer information (connection),
  buy instead send the actual payload in its plain form together with peer domain string and peer port.

  We try to locate the relevant connection info from the bpf map using peer information together with pid as a key
*/
SEC("kprobe/handle_async_payload")
int kprobe_handle_async_payload(struct pt_regs *ctx) {
    const bool val = true;
    u32 bytes_read = 0;

    //interactive pointer to read the data buffer
    void* bufferPtr = GET_DATA_PTR(ctx);

    connection_by_peer_key_t peer_key ={0};
    peer_key.pid = bpf_get_current_pid_tgid() >> 32;

    //read the peer tuple (domain string and port)
    if (0 != bpf_probe_read_user(&peer_key.peer, sizeof(peer_t), bufferPtr)){
        log_debug("[handle_async_payload] failed allocating peer tuple struct on heap\n");
        return 1;
    }
    bufferPtr+=sizeof(peer_t);
    log_debug("[handle_async_payload] pid: %d; peer domain: %s; peer port: %d\n",
         peer_key.pid,
         peer_key.peer.domain,
         peer_key.peer.port);

    //get connection tuple
    conn_tuple_t * actual_connection = bpf_map_lookup_elem(&java_conn_tuple_by_peer, &peer_key);
    if (!actual_connection) {
        log_debug("[handle_async_payload] couldn't correlate connection\n");
        return 1;
    }

    // we need to copy the connection on the stack to be able to call bpf_map_update_elem on old kernels
    conn_tuple_t conn_on_stack = *actual_connection;
    log_debug("[handle_async_payload] found correlation conn src port: %d dst port: %d\n",
             actual_connection->sport,
             actual_connection->dport);

    // read the actual length of the message (limited to HTTP_BUFFER_SIZE bytes)
    if (0 != bpf_probe_read_user(&bytes_read, sizeof(bytes_read), bufferPtr)){
        log_debug("[handle_async_payload] failed reading message length location for pid %d\n", peer_key.pid);
        return 1;
    }
    bufferPtr+=sizeof(bytes_read);

    // register the connection in our map
    bpf_map_update_elem(&java_tls_connections, &conn_on_stack, &val, BPF_ANY);
    log_debug("[handle_async_payload] handling tls request of size: %d for connection src addr: %llx; dst address %llx\n",
                bytes_read,
                actual_connection->saddr_l,
                actual_connection->daddr_l);
    tls_process(ctx, actual_connection, bufferPtr, bytes_read, JAVA_TLS);
    return 0;
}

#endif // __ERPC_HANDLERS_H
