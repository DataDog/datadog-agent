#ifndef __HTTP_MAPS_H
#define __HTTP_MAPS_H

#include "tracer.h"
#include "bpf_helpers.h"
#include "http-types.h"
#include "go-tls-types.h"
#include "map-defs.h"

/* This map is used to keep track of in-flight HTTP transactions for each TCP connection */
BPF_LRU_MAP(http_in_flight, conn_tuple_t, http_transaction_t, 0)

/* This map used for flush complete HTTP batches to userspace */
BPF_PERF_EVENT_ARRAY_MAP(http_batch_events, __u32, 0)

/*
  This map stores finished HTTP transactions in batches so they can be consumed by userspace
  Size is set dynamically during runtime and must be equal to CPUs*HTTP_BATCH_PAGES
 */
BPF_HASH_MAP(http_batches, http_batch_key_t, http_batch_t, 0)

/* This map holds one entry per CPU storing state associated to current http batch*/
BPF_PERCPU_ARRAY_MAP(http_batch_state, __u32, http_batch_state_t, 1)

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
BPF_HASH_MAP(go_tls_read_args, go_tls_function_args_key_t, go_tls_read_args_data_t, 1024)

/* go_tls_write_args is used to get the read function info when running in the write-return uprobe.
   The key contains the go routine id and the pid. */
BPF_HASH_MAP(go_tls_write_args, go_tls_function_args_key_t, go_tls_write_args_data_t, 1024)

/* This map associates crypto/tls.(*Conn) values to the corresponding conn_tuple_t* value.
   It is used to implement a simplified version of tup_from_ssl_ctx from http.c */
BPF_HASH_MAP(conn_tup_by_tls_conn, __u32, conn_tuple_t, 1024)

/* thread_struct id too big for allocation on stack in eBPF function, we use an array as a heap allocator */
BPF_PERCPU_ARRAY_MAP(task_thread, __u32, struct thread_struct, 1)

/* This map used for notifying userspace of a shared library being loaded */
BPF_PERF_EVENT_ARRAY_MAP(shared_libraries, __u32, 0)

#endif
