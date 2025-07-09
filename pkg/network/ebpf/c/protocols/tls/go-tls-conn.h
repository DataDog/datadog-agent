#ifndef __GO_TLS_CONN_H
#define __GO_TLS_CONN_H

#include "bpf_helpers.h"
#include "ip.h"
#include "port_range.h"
#include "pid_tgid.h"

#include "protocols/http/maps.h"
#include "protocols/tls/go-tls-types.h"

typedef struct {
    __u64 ptr;
    __u64 len;
    __u64 cap;
} slice_t;

// Resolve the underlying struct behind the interface pointer.
static __always_inline void* resolve_interface(void *iface_addr) {
    interface_t inner_object;
    if (bpf_probe_read_user(&inner_object, sizeof(inner_object), iface_addr)) {
        log_debug("[go-tls-conn] failed to read interface at %p", iface_addr);
        return NULL;
    }
    return (void*)inner_object.ptr;
}

static __always_inline bool __tuple_via_tcp_conn(tls_conn_layout_t* cl, void* tcp_conn_ptr, conn_tuple_t *output) {
    void* tcp_conn_inner_conn_ptr = tcp_conn_ptr + cl->tcp_conn_inner_conn_offset;
    // the net.conn struct is embedded in net.TCPConn, so just add the offset again
    void* conn_fd_ptr_ptr = tcp_conn_inner_conn_ptr + cl->conn_fd_offset;

    void* conn_fd_ptr;
    if (bpf_probe_read_user(&conn_fd_ptr, sizeof(conn_fd_ptr), conn_fd_ptr_ptr)) {
        return false;
    }

    // Change here
    void *family_ptr = conn_fd_ptr + cl->conn_fd_family_offset;
    __u32 family = 0;
    if (bpf_probe_read_user(&family, sizeof(family), family_ptr)) {
        log_debug("[go-tls-conn] failed to read family from conn_fd_ptr %p", conn_fd_ptr);
        return false;
    }
    log_debug("[go-tls-conn] family: %u", family);

    // read laddr
    void* laddr_interface_ptr = conn_fd_ptr + cl->conn_fd_laddr_offset;
    void *laddr_ptr = resolve_interface(laddr_interface_ptr);
    if (laddr_ptr == NULL) {
        log_debug("[go-tls-conn] failed to resolve laddr interface at %p", laddr_interface_ptr);
        return false;
    } else {
        log_debug("[go-tls-conn] laddr resolved to %p", laddr_ptr);
    }

    __u32 laddr_port = 0;
    if (bpf_probe_read_user(&laddr_port, sizeof(laddr_port), laddr_ptr + cl->tcp_addr_port_offset)) {
        log_debug("[go-tls-conn] failed to read laddr port from %p", laddr_ptr);
        return false;
    } else {
        log_debug("[go-tls-conn] laddr port: %u", laddr_port);
        output->sport = (__u16)laddr_port;
    }

    void* laddr_ip_ptr = laddr_ptr + cl->tcp_addr_ip_offset;
    slice_t laddr_ip_slice = {0};
    if (bpf_probe_read_user(&laddr_ip_slice, sizeof(laddr_ip_slice), laddr_ip_ptr)) {
        log_debug("[go-tls-conn] failed to read laddr IP slice from %p", laddr_ip_ptr);
        return false;
    } else {
        log_debug("[go-tls-conn] laddr IP slice; %p, len: %llu, cap: %llu", (void*)laddr_ip_slice.ptr, laddr_ip_slice.len, laddr_ip_slice.cap);
        if (laddr_ip_slice.len != 4 && laddr_ip_slice.len != 16) {
            log_debug("[go-tls-conn] invalid laddr IP slice length: %llu", laddr_ip_slice.len);
            return false;
        }
        if (laddr_ip_slice.len == 4) {
            char ipv4[4] = {0};
            if (bpf_probe_read_user(&ipv4, sizeof(ipv4), (void*)laddr_ip_slice.ptr)) {
                log_debug("[go-tls-conn] failed to read laddr IPv4 from %p", (void*)laddr_ip_slice.ptr);
                return false;
            } else {
                log_debug("[go-tls-conn] laddr1 IPv4: %u.%u", ipv4[0], ipv4[1]);
                log_debug("[go-tls-conn] laddr2 IPv4: %u.%u", ipv4[2], ipv4[3]);
                output->saddr_h = 0;
                output->saddr_l = *((__u32*)ipv4);
            }
        } else {
            char ipv6[16] = {0};
            if (bpf_probe_read_user(&ipv6, sizeof(ipv6), (void*)laddr_ip_slice.ptr)) {
                log_debug("[go-tls-conn] failed to read laddr IPv6 from %p", (void*)laddr_ip_slice.ptr);
                return false;
            } else {
                log_debug("[go-tls-conn] laddr1 IPv6: %u.%u", ipv6[0], ipv6[1]);
                log_debug("[go-tls-conn] laddr2 IPv6: %u.%u", ipv6[2], ipv6[3]);
                log_debug("[go-tls-conn] laddr3 IPv6: %u.%u", ipv6[4], ipv6[5]);
                log_debug("[go-tls-conn] laddr4 IPv6: %u.%u", ipv6[6], ipv6[7]);
                log_debug("[go-tls-conn] laddr5 IPv6: %u.%u", ipv6[8], ipv6[9]);
                log_debug("[go-tls-conn] laddr6 IPv6: %u.%u", ipv6[10], ipv6[11]);
                log_debug("[go-tls-conn] laddr7 IPv6: %u.%u", ipv6[12], ipv6[13]);
                log_debug("[go-tls-conn] laddr8 IPv6: %u.%u", ipv6[14], ipv6[15]);
                output->saddr_h = *((__u64*)ipv6);
                output->saddr_l = *((__u64*)(ipv6 + 8));
            }
        }
    }

    // read raddr
    void* raddr_interface_ptr = conn_fd_ptr + cl->conn_fd_raddr_offset;
    void *raddr_ptr = resolve_interface(raddr_interface_ptr);
    if (!raddr_ptr) {
        log_debug("[go-tls-conn] failed to resolve raddr interface at %p", raddr_interface_ptr);
        return false;
    } else {
        log_debug("[go-tls-conn] raddr resolved to %p", raddr_ptr);
    }

    __u32 raddr_port = 0;
    if (bpf_probe_read_user(&raddr_port, sizeof(raddr_port), raddr_ptr + cl->tcp_addr_port_offset)) {
        log_debug("[go-tls-conn] failed to read raddr port from %p", raddr_ptr);
        return false;
    } else {
        log_debug("[go-tls-conn] raddr port: %u", raddr_port);
        output->dport = (__u16)raddr_port;
    }

    void* raddr_ip_ptr = raddr_ptr + cl->tcp_addr_ip_offset;
    slice_t raddr_ip_slice = {0};
    if (bpf_probe_read_user(&raddr_ip_slice, sizeof(raddr_ip_slice), raddr_ip_ptr)) {
        log_debug("[go-tls-conn] failed to read raddr IP slice from %p", raddr_ip_ptr);
        return false;
    } else {
        log_debug("[go-tls-conn] raddr IP slice; %p, len: %llu, cap: %llu", (void*)raddr_ip_slice.ptr, raddr_ip_slice.len, raddr_ip_slice.cap);
        if (raddr_ip_slice.len != 4 && raddr_ip_slice.len != 16) {
            log_debug("[go-tls-conn] invalid raddr IP slice length: %llu", raddr_ip_slice.len);
            return false;
        }
        if (raddr_ip_slice.len == 4) {
            char ipv4[4] = {0};
            if (bpf_probe_read_user(&ipv4, sizeof(ipv4), (void*)raddr_ip_slice.ptr)) {
                log_debug("[go-tls-conn] failed to read raddr IPv4 from %p", (void*)raddr_ip_slice.ptr);
                return false;
            } else {
                log_debug("[go-tls-conn] raddr1 IPv4: %u.%u", ipv4[0], ipv4[1]);
                log_debug("[go-tls-conn] raddr2 IPv4: %u.%u", ipv4[2], ipv4[3]);
                output->daddr_h = 0;
                output->daddr_l = *((__u32*)ipv4);
            }
        } else {
            char ipv6[16] = {0};
            if (bpf_probe_read_user(&ipv6, sizeof(ipv6), (void*)raddr_ip_slice.ptr)) {
                log_debug("[go-tls-conn] failed to read raddr IPv6 from %p", (void*)raddr_ip_slice.ptr);
                return false;
            } else {
                log_debug("[go-tls-conn] raddr1 IPv6: %u.%u", ipv6[0], ipv6[1]);
                log_debug("[go-tls-conn] raddr2 IPv6: %u.%u", ipv6[2], ipv6[3]);
                log_debug("[go-tls-conn] raddr3 IPv6: %u.%u", ipv6[4], ipv6[5]);
                log_debug("[go-tls-conn] raddr4 IPv6: %u.%u", ipv6[6], ipv6[7]);
                log_debug("[go-tls-conn] raddr5 IPv6: %u.%u", ipv6[8], ipv6[9]);
                log_debug("[go-tls-conn] raddr6 IPv6: %u.%u", ipv6[10], ipv6[11]);
                log_debug("[go-tls-conn] raddr7 IPv6: %u.%u", ipv6[12], ipv6[13]);
                log_debug("[go-tls-conn] raddr8 IPv6: %u.%u", ipv6[14], ipv6[15]);
                output->daddr_h = *((__u64*)ipv6);
                output->daddr_l = *((__u64*)(ipv6 + 8));
            }
        }
    }

    return true;
}

//static __always_inline conn_tuple_t* __tuple_via_limited_conn(tls_conn_layout_t* cl, void* limited_conn_ptr, pid_fd_t* pid_fd) {
//    void *net_conn_ptr = limited_conn_ptr + cl->limited_conn_inner_conn_offset;
//
//    interface_t inner_conn_iface;
//    if (bpf_probe_read_user(&inner_conn_iface, sizeof(inner_conn_iface), net_conn_ptr)) {
//        return NULL;
//    }
//
//    return __tuple_via_tcp_conn(cl, (void *)inner_conn_iface.ptr, pid_fd);
//}

static __always_inline conn_tuple_t* conn_tup_from_tls_conn(tls_offsets_data_t* pd, void* conn, uint64_t pid_tgid) {
    conn_tuple_t* tup = bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, &conn);
    if (tup != NULL) {
        log_debug("[go-tls-conn] found existing tuple for conn %p", conn);
        return tup;
    }

    // The tls.Conn struct has a `conn` field of type `net.Conn` (interface)
    // Here we obtain the pointer to the concrete type behind this interface.
    void* tls_conn_inner_conn_ptr = conn + pd->conn_layout.tls_conn_inner_conn_offset;
    interface_t inner_conn_iface;
    if (bpf_probe_read_user(&inner_conn_iface, sizeof(inner_conn_iface), tls_conn_inner_conn_ptr)) {
        return NULL;
    }

    conn_tuple_t tuple;
    if (!__tuple_via_tcp_conn(&pd->conn_layout, (void *)inner_conn_iface.ptr, &tuple)) {
        log_debug("[go-tls-conn] failed to create tuple from tcp conn at %p", (void *)inner_conn_iface.ptr);
        return NULL;
    }

    bpf_map_update_elem(&conn_tup_by_go_tls_conn, &conn, &tuple, BPF_ANY);
    return bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, &conn);
}

#endif //__GO_TLS_CONN_H
