#ifndef __GO_TLS_MAPS_H
#define __GO_TLS_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "protocols/tls/go-tls-types.h"

// offsets_data map contains the information about the locations of structs in the inspected binary, mapped by the binary's inode number.
BPF_HASH_MAP(offsets_data, go_tls_offsets_data_key_t, tls_offsets_data_t, 1024)

/* go_tls_read_args is used to get the read function info when running in the read-return uprobe.
   The key contains the go routine id and the pid. */
BPF_HASH_MAP(go_tls_read_args, go_tls_function_args_key_t, go_tls_read_args_data_t, 10240)

/* go_tls_write_args is used to get the read function info when running in the write-return uprobe.
   The key contains the go routine id and the pid. */
BPF_HASH_MAP(go_tls_write_args, go_tls_function_args_key_t, go_tls_write_args_data_t, 10240)

/* This map associates crypto/tls.(*Conn) values to the corresponding conn_tuple_t* value.
   It is used to implement a simplified version of tup_from_ssl_ctx from usm.c
   Map size is set to 1 as goTLS is optional, this will be overwritten to MaxTrackedConnections
   if goTLS is enabled. */
BPF_HASH_MAP(conn_tup_by_go_tls_conn, void*, conn_tuple_t, 1)

/* This map associates conn_tuple_t values to the corresponding crypto/tls.(*Conn) pointer.
   It is used to clean up the conn_tup_by_go_tls_conn map when TCP connections are closed.
   Map size is set to 1 as goTLS is optional, this will be overwritten to MaxTrackedConnections
   if goTLS is enabled. */
BPF_HASH_MAP(go_tls_conn_by_tuple, conn_tuple_t, void*, 1)


/* gotls_dispatch_progs holds the five GoTLS probe bodies, indexed by the attach cookie
   that uprobe__crypto_tls_Conn_dispatch reads. A uprobe_multi link binds to exactly one
   program, so attaching the five probes separately costs five links per binary, and a
   link close pays the same RCU grace period as a perf_event fd close. Dispatching through
   this array lets a single link cover all five. */
BPF_PROG_ARRAY(gotls_dispatch_progs, GOTLS_DISPATCH_MAX)

#endif //__GO_TLS_MAPS_H
