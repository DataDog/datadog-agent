#ifndef __HTTP_MAPS_H
#define __HTTP_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"
#include "tracer.h"

#include "protocols/http/types.h"
#include "protocols/tls/go-tls-types.h"

/* This map is used to keep track of in-flight HTTP transactions for each TCP connection */
BPF_LRU_MAP(http_in_flight, conn_tuple_t, http_transaction_t, 0)

BPF_LRU_MAP(ssl_sock_by_ctx, void *, ssl_sock_t, 1)

BPF_LRU_MAP(ssl_read_args, u64, ssl_read_args_t, 1024)

BPF_LRU_MAP(ssl_read_ex_args, u64, ssl_read_ex_args_t, 1024)

BPF_LRU_MAP(ssl_write_args, u64, ssl_write_args_t, 1024)

BPF_LRU_MAP(ssl_write_ex_args, u64, ssl_write_ex_args_t, 1024)

BPF_LRU_MAP(bio_new_socket_args, __u64, __u32, 1024)

BPF_LRU_MAP(fd_by_ssl_bio, __u32, void *, 1024)

BPF_LRU_MAP(ssl_ctx_by_pid_tgid, __u64, void *, 1024)

BPF_LRU_MAP(open_at_args, __u64, lib_path_t, 1024)

// offsets_data map contains the information about the locations of structs in the inspected binary, mapped by the binary's inode number.
BPF_HASH_MAP(offsets_data, go_tls_offsets_data_key_t, tls_offsets_data_t, 1024)

/* go_tls_read_args is used to get the read function info when running in the read-return uprobe.
   The key contains the go routine id and the pid. */
BPF_LRU_MAP(go_tls_read_args, go_tls_function_args_key_t, go_tls_read_args_data_t, 2048)

/* go_tls_write_args is used to get the read function info when running in the write-return uprobe.
   The key contains the go routine id and the pid. */
BPF_LRU_MAP(go_tls_write_args, go_tls_function_args_key_t, go_tls_write_args_data_t, 2048)

/* This map associates crypto/tls.(*Conn) values to the corresponding conn_tuple_t* value.
   It is used to implement a simplified version of tup_from_ssl_ctx from usm.c
   Map size is set to 1 as goTLS is optional, this will be overwritten to MaxTrackedConnections
   if goTLS is enabled. */
BPF_HASH_MAP(conn_tup_by_go_tls_conn, __u32, conn_tuple_t, 1)

// A set (map from a key to a const bool value, we care only if the key exists in the map, and not its value) to
// mark if we've seen a specific java tls connection.
BPF_LRU_MAP(java_tls_connections, conn_tuple_t, bool, 1)

/* This map used for notifying userspace of a shared library being loaded */
BPF_PERF_EVENT_ARRAY_MAP(shared_libraries, __u32)

#endif
