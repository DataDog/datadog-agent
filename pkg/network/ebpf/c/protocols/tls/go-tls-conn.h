#ifndef __GO_TLS_CONN_H
#define __GO_TLS_CONN_H

#include "bpf_helpers.h"
#include "ip.h"
#include "port_range.h"

#include "protocols/http/maps.h"
#include "protocols/tls/go-tls-types.h"

static __always_inline conn_tuple_t* __tuple_via_tcp_conn(tls_conn_layout_t* cl, void* tcp_conn_ptr, pid_fd_t* pid_fd) {
    void* tcp_conn_inner_conn_ptr = tcp_conn_ptr + cl->tcp_conn_inner_conn_offset;
    // the net.conn struct is embedded in net.TCPConn, so just add the offset again
    void* conn_fd_ptr_ptr = tcp_conn_inner_conn_ptr + cl->conn_fd_offset;

    void* conn_fd_ptr;
    if (bpf_probe_read_user(&conn_fd_ptr, sizeof(conn_fd_ptr), conn_fd_ptr_ptr)) {
        return NULL;
    }

    void* net_fd_pfd_ptr = conn_fd_ptr + cl->net_fd_pfd_offset;
    // the internal/poll.FD struct is embedded in net.netFD, so just add the offset again
    void* fd_sysfd_ptr = net_fd_pfd_ptr + cl->fd_sysfd_offset;

    // dereference the pointer to get the file descriptor
    if (bpf_probe_read_user(&pid_fd->fd, sizeof(pid_fd->fd), fd_sysfd_ptr)) {
        return NULL;
    }

    return bpf_map_lookup_elem(&tuple_by_pid_fd, pid_fd);
}

static __always_inline conn_tuple_t* __tuple_via_limited_conn(tls_conn_layout_t* cl, void* limited_conn_ptr, pid_fd_t* pid_fd) {
    void *net_conn_ptr = limited_conn_ptr + cl->limited_conn_inner_conn_offset;

    interface_t inner_conn_iface;
    if (bpf_probe_read_user(&inner_conn_iface, sizeof(inner_conn_iface), net_conn_ptr)) {
        return NULL;
    }

    return __tuple_via_tcp_conn(cl, (void *)inner_conn_iface.ptr, pid_fd);
}

static __always_inline conn_tuple_t* conn_tup_from_tls_conn(tls_offsets_data_t* pd, void* conn, uint64_t pid_tgid) {
    conn_tuple_t* tup = bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, &conn);
    if (tup != NULL) {
        return tup;
    }

    // The code path below should be executed only once during the lifecycle of a TLS connection
    pid_fd_t pid_fd = {
        .pid = pid_tgid >> 32,
        // fd is populated by the code downstream
        .fd = 0,
    };

    // The tls.Conn struct has a `conn` field of type `net.Conn` (interface)
    // Here we obtain the pointer to the concrete type behind this interface.
    void* tls_conn_inner_conn_ptr = conn + pd->conn_layout.tls_conn_inner_conn_offset;
    interface_t inner_conn_iface;
    if (bpf_probe_read_user(&inner_conn_iface, sizeof(inner_conn_iface), tls_conn_inner_conn_ptr)) {
        return NULL;
    }

    conn_tuple_t *tuple = __tuple_via_tcp_conn(&pd->conn_layout, (void *)inner_conn_iface.ptr, &pid_fd);
    if (!tuple) {
        tuple = __tuple_via_limited_conn(&pd->conn_layout, (void *)inner_conn_iface.ptr, &pid_fd);
    }

    if (!tuple) {
        return NULL;
    }

    // Copy tuple to stack before inserting it in another map (necessary for older Kernels)
    conn_tuple_t tuple_copy;
    bpf_memcpy(&tuple_copy, tuple, sizeof(conn_tuple_t));

    bpf_map_update_elem(&conn_tup_by_go_tls_conn, &conn, &tuple_copy, BPF_ANY);
    return bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, &conn);
}

#endif //__GO_TLS_CONN_H
