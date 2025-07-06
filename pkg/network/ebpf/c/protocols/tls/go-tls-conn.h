#ifndef __GO_TLS_CONN_H
#define __GO_TLS_CONN_H

#include "bpf_helpers.h"
#include "ip.h"
#include "port_range.h"
#include "pid_tgid.h"

#include "protocols/http/maps.h"
#include "protocols/tls/go-tls-types.h"
#include "protocols/sockfd.h"
#include "sock.h"

static __always_inline conn_tuple_t* __tuple_via_tcp_conn_direct(tls_conn_layout_t* cl, void* tcp_conn_ptr, uint64_t pid_tgid) {
    log_debug("[go-tls-conn] Extracting tuple from TCPConn at %p", tcp_conn_ptr);
    
    // Navigate through the Go struct hierarchy to get the file descriptor
    // TCPConn -> conn -> fd -> netFD -> pfd -> Sysfd
    
    // Get the inner conn from TCPConn
    void* conn_ptr;
    if (bpf_probe_read_user(&conn_ptr, sizeof(conn_ptr), tcp_conn_ptr + cl->tcp_conn_inner_conn_offset)) {
        log_debug("[go-tls-conn] Failed to read conn from TCPConn");
        return NULL;
    }
    
    // Get the netFD from conn
    void* netfd_ptr;
    if (bpf_probe_read_user(&netfd_ptr, sizeof(netfd_ptr), conn_ptr + cl->conn_fd_offset)) {
        log_debug("[go-tls-conn] Failed to read netFD from conn");
        return NULL;
    }
    
    // Get the poll.FD from netFD
    void* pfd_ptr = netfd_ptr + cl->net_fd_pfd_offset;
    
    // Get the system file descriptor
    int sysfd;
    if (bpf_probe_read_user(&sysfd, sizeof(sysfd), pfd_ptr + cl->fd_sysfd_offset)) {
        log_debug("[go-tls-conn] Failed to read sysfd from pfd");
        return NULL;
    }
    
    log_debug("[go-tls-conn] Extracted sysfd: %d", sysfd);
    
    // Create pid_fd key for lookup
    pid_fd_t pid_fd = {\n        .pid = GET_USER_MODE_PID(pid_tgid),\n        .fd = sysfd,\n    };
    
    // Look up the connection tuple from the socket file descriptor
    conn_tuple_t* tuple = bpf_map_lookup_elem(&tuple_by_pid_fd, &pid_fd);
    if (!tuple) {
        log_debug("[go-tls-conn] Failed to get tuple from socket fd");
        return NULL;
    }
    
    log_debug("[go-tls-conn] Successfully extracted tuple");
    
    // Store the tuple in the map
    bpf_map_update_elem(&conn_tup_by_go_tls_conn, tuple, tuple, BPF_ANY);
    return bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, tuple);
}

static __always_inline conn_tuple_t* __tuple_via_tcp_conn(tls_conn_layout_t* cl, void* tcp_conn_ptr, uint64_t pid_tgid) {
    return __tuple_via_tcp_conn_direct(cl, tcp_conn_ptr, pid_tgid);
}

static __always_inline conn_tuple_t* __tuple_via_go_tls_conn(tls_conn_layout_t* cl, void* go_tls_conn_ptr, uint64_t pid_tgid) {
    // Get the inner net.Conn from crypto/tls.Conn
    void* inner_conn_ptr;
    if (bpf_probe_read_user(&inner_conn_ptr, sizeof(inner_conn_ptr), go_tls_conn_ptr + cl->tls_conn_inner_conn_offset)) {
        log_debug("[go-tls-conn] Failed to read inner conn from tls.Conn");
        return NULL;
    }
    
    // The inner conn should be a *net.TCPConn, so we can use our TCPConn extraction
    return __tuple_via_tcp_conn_direct(cl, inner_conn_ptr, pid_tgid);
}

static __always_inline conn_tuple_t* conn_tup_from_tls_conn(tls_offsets_data_t* pd, void* conn, uint64_t pid_tgid) {
    conn_tuple_t* tup = bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, &conn);
    if (tup != NULL) {
        return tup;
    }

    // The tls.Conn struct has a `conn` field of type `net.Conn` (interface)
    // Here we obtain the pointer to the concrete type behind this interface.
    void* tls_conn_inner_conn_ptr = conn + pd->conn_layout.tls_conn_inner_conn_offset;
    interface_t inner_conn_iface;
    if (bpf_probe_read_user(&inner_conn_iface, sizeof(inner_conn_iface), tls_conn_inner_conn_ptr)) {
        return NULL;
    }

    // Extract connection tuple from TCPConn
    conn_tuple_t *tuple = __tuple_via_tcp_conn(&pd->conn_layout, (void *)inner_conn_iface.ptr, pid_tgid);
    if (!tuple) {
        return NULL;
    }

    // Set the PID from the current context
    tuple->pid = GET_USER_MODE_PID(pid_tgid);

    // Copy tuple to stack before inserting it in another map (necessary for older Kernels)
    conn_tuple_t tuple_copy;
    bpf_memcpy(&tuple_copy, tuple, sizeof(conn_tuple_t));

    bpf_map_update_elem(&conn_tup_by_go_tls_conn, &conn, &tuple_copy, BPF_ANY);
    return bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, &conn);
}

#endif //__GO_TLS_CONN_H
