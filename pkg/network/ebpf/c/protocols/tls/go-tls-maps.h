#ifndef __GO_TLS_MAPS_H
#define __GO_TLS_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "protocols/tls/go-tls-types.h"

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

#endif //__GO_TLS_MAPS_H
