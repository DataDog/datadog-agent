#ifndef __GO_TLS_CONN_H
#define __GO_TLS_CONN_H

#include "bpf_helpers.h"
#include "ip.h"
#include "port_range.h"
#include "pid_tgid.h"

#include "protocols/http/maps.h"
#include "protocols/tls/go-tls-types.h"

// Resolves the underlying struct behind the interface pointer.
static __always_inline void* resolve_interface(void *iface_addr) {
    interface_t inner_object;
    if (bpf_probe_read_user(&inner_object, sizeof(inner_object), iface_addr)) {
        return NULL;
    }
    return (void*)inner_object.ptr;
}

#define IPV4_ADDR_LEN 4
#define IPV6_ADDR_LEN 16
#define IP_ADDR_LEN_MAX IPV6_ADDR_LEN

// Reads an IP address from the provided slice_t structure.
static __always_inline bool read_ip(slice_t *address_ptr, __u32 family, __u64 *out_address_h, __u64 *out_address_l) {
    // Taking 16 bytes as it is the maximum size for an IPv6 address (128 bits) and an IPv4 address (32 bits) can fit in this space.
    char ip[IP_ADDR_LEN_MAX] = {0};
    if (address_ptr->len == IPV4_ADDR_LEN && family == AF_INET) {
        if (bpf_probe_read_user(ip, IPV4_ADDR_LEN, (void*)address_ptr->ptr) == 0) {
            *out_address_h = 0;
            *out_address_l = *((__u32*)ip);
            return true;
        }
    } else if (address_ptr->len == IPV6_ADDR_LEN && family == AF_INET6) {
        if (bpf_probe_read_user(ip, IPV6_ADDR_LEN, (void*)address_ptr->ptr) == 0) {
            *out_address_h = *((__u64*)ip);
            *out_address_l = *((__u64*)(ip + 8));
            return true;
        }
    }
    log_debug("[go-tls-conn] invalid address length: %llu; or invalid family: %u", address_ptr->len, family);
    return false;
}

// Reads the port from the provided pointer into the out_port variable.
static __always_inline bool read_port(void *ptr, __u16 *out_port) {
    // Since the port is stored as int, we have to read it as 32 bits and then cast it to 16 bits.
    __u32 port = 0;
    if (bpf_probe_read_user(&port, sizeof(__u16), ptr)) {
        log_debug("[go-tls-conn] failed to read port at %p", ptr);
        return false;
    }
    *out_port = (__u16)port;
    return true;
}

static __always_inline bool __tuple_via_tcp_conn(tls_conn_layout_t* cl, void* tcp_conn_ptr, conn_tuple_t *output) {
    void* conn_fd_ptr;
    if (bpf_probe_read_user(&conn_fd_ptr, sizeof(conn_fd_ptr), tcp_conn_ptr + cl->tcp_conn_inner_conn_offset + cl->conn_fd_offset)) {
        return false;
    }

    __u32 family = 0;
    if (bpf_probe_read_user(&family, sizeof(family), conn_fd_ptr + cl->conn_fd_family_offset)) {
        return false;
    }

    // read laddr
    void *addr_ptr = resolve_interface(conn_fd_ptr + cl->conn_fd_laddr_offset);
    if (addr_ptr == NULL) {
        return false;
    }

    if (!read_port(addr_ptr + cl->tcp_addr_port_offset, &output->sport)) {
        return false;
    }

    slice_t addr_ip_slice = {0};
    if (bpf_probe_read_user(&addr_ip_slice, sizeof(slice_t), addr_ptr + cl->tcp_addr_ip_offset)) {
        return false;
    }

    if (!read_ip(&addr_ip_slice, family, &output->saddr_h, &output->saddr_l)) {
        return false;
    }

    // read raddr
    addr_ptr = resolve_interface(conn_fd_ptr + cl->conn_fd_raddr_offset);
    if (addr_ptr == NULL) {
        return false;
    }

    if (!read_port(addr_ptr + cl->tcp_addr_port_offset, &output->dport)) {
        return false;
    }

    if (bpf_probe_read_user(&addr_ip_slice, sizeof(slice_t), addr_ptr + cl->tcp_addr_ip_offset)) {
        return false;
    }

    if (!read_ip(&addr_ip_slice, family, &output->daddr_h, &output->daddr_l)) {
        return false;
    }

    // Similar behavior as in read_conn_tuple_partial
    // See documentation of is_ipv4_mapped_ipv6 for further details.
    if (family == AF_INET6) {
        // Check if we can map IPv6 to IPv4
        if (is_ipv4_mapped_ipv6(output->saddr_h, output->saddr_l, output->daddr_h, output->daddr_l)) {
            output->metadata |= CONN_V4;
            output->saddr_h = 0;
            output->daddr_h = 0;
            output->saddr_l = (__u32)(output->saddr_l >> 32);
            output->daddr_l = (__u32)(output->daddr_l >> 32);
        } else {
            output->metadata |= CONN_V6;
        }
    } else {
        output->metadata |= CONN_V4;
    }
    return true;
}

static __always_inline bool __tuple_via_limited_conn(tls_conn_layout_t* cl, void* limited_conn_ptr, conn_tuple_t *output) {
    void *inner_conn_iface_ptr = resolve_interface(limited_conn_ptr + cl->limited_conn_inner_conn_offset);
    if (inner_conn_iface_ptr == NULL) {
        return false;
    }

    return __tuple_via_tcp_conn(cl, inner_conn_iface_ptr, output);
}

static __always_inline conn_tuple_t* conn_tup_from_tls_conn(tls_offsets_data_t* pd, void* conn) {
    conn_tuple_t* tup = bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, &conn);
    if (tup != NULL) {
        return tup;
    }

    // The tls.Conn struct has a `conn` field of type `net.Conn` (interface)
    // Here we obtain the pointer to the concrete type behind this interface.
    void *inner_conn_iface_ptr = resolve_interface(conn + pd->conn_layout.tls_conn_inner_conn_offset);
    if (inner_conn_iface_ptr == NULL) {
        return NULL;
    }

    conn_tuple_t tuple = {
        .pid = GET_USER_MODE_PID(bpf_get_current_pid_tgid()),
        .metadata = CONN_TYPE_TCP,
    };

    struct task_struct *task = (struct task_struct *) bpf_get_current_task();
    tuple.netns = BPF_CORE_READ(task, nsproxy, net_ns, ns.inum);

    if (!__tuple_via_tcp_conn(&pd->conn_layout, inner_conn_iface_ptr, &tuple)) {
        if (!__tuple_via_limited_conn(&pd->conn_layout, inner_conn_iface_ptr, &tuple)) {
            return NULL;
        }
    }

    bpf_map_update_with_telemetry(conn_tup_by_go_tls_conn, &conn, &tuple, BPF_ANY);
    bpf_map_update_with_telemetry(go_tls_conn_by_tuple, &tuple, &conn, BPF_ANY);
    return bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, &conn);
}

#endif //__GO_TLS_CONN_H
