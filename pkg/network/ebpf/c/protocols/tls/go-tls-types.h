#ifndef __GO_TLS_TYPES_H
#define __GO_TLS_TYPES_H

#include "ktypes.h"

typedef struct {
    __s64 stack_offset;
    __s64 _register;
    __u8 in_register;
    __u8 exists;
} location_t;

typedef struct {
    location_t ptr;
    location_t len;
    location_t cap;
} slice_location_t;

// equivalent to runtime.iface
// https://golang.org/src/runtime/runtime2.go
typedef struct {
    __u64 itab;
    __u64 ptr;
} interface_t;

// equivalent to unsafe.SliceHeader.
typedef struct {
    __u64 ptr;
    __u64 len;
    __u64 cap;
} slice_t;

typedef struct {
    __u64 runtime_g_tls_addr_offset;
    __u64 goroutine_id_offset;
    __s64 runtime_g_register;
    __u8 runtime_g_in_register;
} goroutine_id_metadata_t;

typedef struct {
    __u64 tls_conn_inner_conn_offset;
    __u64 tcp_conn_inner_conn_offset;
    __u64 limited_conn_inner_conn_offset;
    __u64 conn_fd_offset;

    // Offset of the field `family` in `netFD` struct.
    __u64 conn_fd_family_offset;
    // Offset of the field `laddr` in `netFD` struct.
    __u64 conn_fd_laddr_offset;
    // Offset of the field `raddr` in `netFD` struct.
    __u64 conn_fd_raddr_offset;
    // Offset of the field `port` in `net.TCPAddr` struct.
    __u64 tcp_addr_port_offset;
    // Offset of the field `ip` in `net.TCPAddr` struct.
    __u64 tcp_addr_ip_offset;
} tls_conn_layout_t;

typedef struct {
    __u32 device_id_major;
    __u32 device_id_minor;
    __u64 ino;
} go_tls_offsets_data_key_t;

typedef struct {
    goroutine_id_metadata_t goroutine_id;
    tls_conn_layout_t conn_layout;

    // func (c *Conn) Read(b []byte) (int, error)
    location_t read_conn_pointer;
    slice_location_t read_buffer;
    location_t read_return_bytes;

    // func (c *Conn) Write(b []byte) (int, error)
    location_t write_conn_pointer;
    slice_location_t write_buffer;
    location_t write_return_bytes;
    location_t write_return_error;

    // func (c *Conn) Close() error
    location_t close_conn_pointer;
} tls_offsets_data_t;

typedef struct {
    __s64 goroutine_id;
    __u32 pid;
} go_tls_function_args_key_t;

typedef struct {
    __u64 conn_pointer;
    __u64 b_data;
} go_tls_read_args_data_t;

typedef struct {
    __u64 conn_pointer;
    __u64 b_data;
    __u64 b_len;
} go_tls_write_args_data_t;

#endif //__GO_TLS_TYPES_H
