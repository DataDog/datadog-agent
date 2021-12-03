#ifndef __GO_TLS_CONN_H
#define __GO_TLS_CONN_H

#include "bpf_helpers.h"
#include "go-tls-types.h"
#include "go-tls-maps.h"
#include "ip.h"

// TODO de-duplicate with http.c
// TODO: Replace those by injected constants based on system configuration
// once we have port range detection merged into the codebase.
#define EPHEMERAL_RANGE_BEG 32768
#define EPHEMERAL_RANGE_END 60999

static __always_inline int is_ephemeral_port(u16 port) {
    return port >= EPHEMERAL_RANGE_BEG && port <= EPHEMERAL_RANGE_END;
}

// Implemented either in c/runtime/conn-tuple.h or from c/sockfd.h
static int read_conn_tuple(conn_tuple_t *t, struct sock *skp, u64 pid_tgid, metadata_mask_t type);

static __always_inline int get_conn_fd(tls_conn_layout_t* cl, void* tls_conn_ptr, int32_t* dest) {
	void* tls_conn_inner_conn_ptr = tls_conn_ptr + cl->tls_conn_inner_conn_offset;

	interface_t inner_conn_iface;
	if (bpf_probe_read(&inner_conn_iface, sizeof(inner_conn_iface), tls_conn_inner_conn_ptr)) {
		return 1;
	}

	// Check that the itab entry for the (net.TCPConn,net.Conn) (type,interface) pair
	// is the same as the tab pointer in the interface.
	// However, this check is only possible if the binary wasn't stripped.
	// If the interface type is 0, skip the check.
	if (cl->tcp_conn_interface_type && ((uint64_t) inner_conn_iface.itab) != cl->tcp_conn_interface_type) {
		return 1;
	}

	void* tcp_conn_inner_conn_ptr = ((void*) inner_conn_iface.ptr) + cl->tcp_conn_inner_conn_offset;
	// the net.conn struct is embedded in net.TCPConn, so just add the offset again
	void* conn_fd_ptr_ptr = tcp_conn_inner_conn_ptr + cl->conn_fd_offset;

	void* conn_fd_ptr;
	if (bpf_probe_read(&conn_fd_ptr, sizeof(conn_fd_ptr), conn_fd_ptr_ptr)) {
		return 1;
	}

	void* net_fd_pfd_ptr = conn_fd_ptr + cl->net_fd_pfd_offset;
	// the internal/poll.FD struct is embedded in net.netFD, so just add the offset again
	void* fd_sysfd_ptr = net_fd_pfd_ptr + cl->fd_sysfd_offset;

	// Finally, dereference the pointer to get the file descriptor
	return bpf_probe_read(dest, sizeof(*dest), fd_sysfd_ptr);
}

static __always_inline conn_tuple_t* conn_tup_from_tls_conn(tls_probe_data_t* pd, void* conn, uint64_t pid_tgid) {
    conn_tuple_t* tup = bpf_map_lookup_elem(&conn_tup_by_tls_conn, &conn);
    if (tup != NULL) {
        return tup;
    }

    // The code path below should be executed only once during the lifecycle of a TLS connection
	int32_t fd;
	if (get_conn_fd(&pd->conn_layout, conn, &fd)) {
		return NULL;
	}
    pid_fd_t pid_fd = {
        .pid = pid_tgid >> 32,
        .fd = fd,
    };

    struct sock **sock = bpf_map_lookup_elem(&sock_by_pid_fd, &pid_fd);
    if (sock == NULL)  {
        return NULL;
    }

    conn_tuple_t t = {0};
    if (!read_conn_tuple(&t, *sock, pid_tgid, CONN_TYPE_TCP)) {
        return NULL;
    }

    // Set the `.netns` and `.pid` values to always be 0.
    // They can't be sourced from inside `read_conn_tuple_skb`,
    // which is used elsewhere to produce the same `conn_tuple_t` value from a `struct __sk_buff*` value,
    // so we ensure it is always 0 here so that both paths produce the same `conn_tuple_t` value.
    // `netns` is not used in the userspace program part that binds http information to `ConnectionStats`,
    // so this is isn't a problem.
    t.netns = 0;
    t.pid = 0;

    if (!is_ephemeral_port(t.sport)) {
        flip_tuple(&t);
    }

	bpf_map_update_elem(&conn_tup_by_tls_conn, &conn, &t, BPF_ANY);
	return bpf_map_lookup_elem(&conn_tup_by_tls_conn, &conn);
}

#endif //__GO_TLS_CONN_H
