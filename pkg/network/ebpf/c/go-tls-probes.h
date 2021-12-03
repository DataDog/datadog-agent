#ifndef __GO_TLS_PROBES_H
#define __GO_TLS_PROBES_H

#include "bpf_helpers.h"
#include "go-tls-types.h"
#include "go-tls-goid.h"
#include "go-tls-location.h"
#include "go-tls-conn.h"
#include "go-tls-maps.h"
#include "http-types.h"
#include "tags-types.h"
#include "http-buffer.h"
#include "http.h"

static __always_inline tls_probe_data_t* get_probe_data() {
	uint32_t key = 0;
	return bpf_map_lookup_elem(&probe_data, &key);
}

// func (c *Conn) Write(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Write")
int uprobe__crypto_tls_Conn_Write(struct pt_regs *ctx) {
	tls_probe_data_t* pd = get_probe_data();
	if (pd == NULL)
		return 1;

	void* conn_pointer = NULL;
	if (read_location(ctx, &pd->write_conn_pointer, sizeof(conn_pointer), &conn_pointer)) {
		return 1;
	}

	void* b_data = NULL;
	if (read_location(ctx, &pd->write_buffer.ptr, sizeof(b_data), &b_data)) {
		return 1;
	}
	uint64_t b_len = 0;
	if (read_location(ctx, &pd->write_buffer.len, sizeof(b_len), &b_len)) {
		return 1;
	}

	u64 pid_tgid = bpf_get_current_pid_tgid();
	conn_tuple_t* t = conn_tup_from_tls_conn(pd, conn_pointer, pid_tgid);
	if (t == NULL) {
		return 1;
	}

    char buffer[HTTP_BUFFER_SIZE];
    __builtin_memset(buffer, 0, sizeof(buffer));
	read_into_buffer(buffer, b_data, b_len);

    skb_info_t skb_info = {0};
    __builtin_memcpy(&skb_info.tup, t, sizeof(conn_tuple_t));
    http_process(buffer, &skb_info, skb_info.tup.sport, GO);

    return 0;
}

// func (c *Conn) Read(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Read")
int uprobe__crypto_tls_Conn_Read(struct pt_regs *ctx) {
	tls_probe_data_t* pd = get_probe_data();
	if (pd == NULL)
		return 1;

	// Read the TGID and goroutine ID to make the partial call key
	read_partial_call_key_t call_key = {0};
	uint64_t pid_tgid = bpf_get_current_pid_tgid();
	call_key.tgid = pid_tgid >> 32;
	if (read_goroutine_id(ctx, &pd->goroutine_id, &call_key.goroutine_id)) {
		return 1;
	}

	// Read the parameters to make the partial call data
	// (since the parameters might not be live by the time the return probe is hit).
	read_partial_call_data_t call_data = {0};
	if (read_location(ctx, &pd->read_conn_pointer, sizeof(call_data.conn_pointer), &call_data.conn_pointer)) {
		return 1;
	}
	if (read_location(ctx, &pd->read_buffer.ptr, sizeof(call_data.b_data), &call_data.b_data)) {
		return 1;
	}

	bpf_map_update_elem(&read_partial_calls, &call_key, &call_data, BPF_ANY);

	return 0;
}

// func (c *Conn) Read(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Read/return")
int uprobe__crypto_tls_Conn_Read__return(struct pt_regs *ctx) {
	tls_probe_data_t* pd = get_probe_data();
	if (pd == NULL)
		return 1;

	// Read the TGID and goroutine ID to make the partial call key
	read_partial_call_key_t call_key = {0};
	uint64_t pid_tgid = bpf_get_current_pid_tgid();
	call_key.tgid = pid_tgid >> 32;
	if (read_goroutine_id(ctx, &pd->goroutine_id, &call_key.goroutine_id)) {
		return 1;
	}

	read_partial_call_data_t* call_data_ptr = bpf_map_lookup_elem(&read_partial_calls, &call_key);
	if (call_data_ptr == NULL) {
		return 1;
	}
	read_partial_call_data_t call_data = *call_data_ptr;
    bpf_map_delete_elem(&read_partial_calls, &call_key);

	uint64_t bytes_read = 0;
	if (read_location(ctx, &pd->read_return_bytes, sizeof(bytes_read), &bytes_read)) {
		return 1;
	}

	conn_tuple_t* t = conn_tup_from_tls_conn(pd, (void*) call_data.conn_pointer, pid_tgid);
	if (t == NULL) {
		return 1;
	}

	// The error return value of Read isn't useful here
	// unless we can determine whether it is equal to io.EOF
	// (and if so, treat it like there's no error at all),
	// and I didn't find a straightforward way of doing this.

    char buffer[HTTP_BUFFER_SIZE];
    __builtin_memset(buffer, 0, sizeof(buffer));
	read_into_buffer(buffer, (char*) call_data.b_data, bytes_read);

    skb_info_t skb_info = {0};
    __builtin_memcpy(&skb_info.tup, t, sizeof(conn_tuple_t));
    http_process(buffer, &skb_info, skb_info.tup.sport, GO);

	return 0;
}

// func (c *Conn) Close(b []byte) (int, error)
SEC("uprobe/crypto/tls.(*Conn).Close")
int uprobe__crypto_tls_Conn_Close(struct pt_regs *ctx) {
	tls_probe_data_t* pd = get_probe_data();
	if (pd == NULL)
		return 1;

	void* conn_pointer = NULL;
	if (read_location(ctx, &pd->close_conn_pointer, sizeof(conn_pointer), &conn_pointer)) {
		return 1;
	}

	u64 pid_tgid = bpf_get_current_pid_tgid();
	conn_tuple_t* t = conn_tup_from_tls_conn(pd, conn_pointer, pid_tgid);
	if (t == NULL) {
		return 1;
	}

    char buffer[HTTP_BUFFER_SIZE];
    __builtin_memset(buffer, 0, sizeof(buffer));

    skb_info_t skb_info = {0};
    __builtin_memcpy(&skb_info.tup, t, sizeof(conn_tuple_t));

    // TODO: this is just a hack. Let's get rid of this skb_info argument altogether
    skb_info.tcp_flags |= TCPHDR_FIN;
    http_process(buffer, &skb_info, skb_info.tup.sport, GO);

	// Clear the element in the map since this connection is closed
    bpf_map_delete_elem(&conn_tup_by_tls_conn, &conn_pointer);

    return 0;
}

#endif //__GO_TLS_PROBES_H
