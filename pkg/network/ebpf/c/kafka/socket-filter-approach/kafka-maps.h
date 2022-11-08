#ifndef __KAFKA_MAPS_H
#define __KAFKA_MAPS_H

//#include "tracer.h"
//#include "bpf_helpers.h"
#include "kafka-types.h"
//#include "map-defs.h"
//
///* This map is used to keep track of in-flight Kafka transactions for each TCP connection */
BPF_LRU_MAP(kafka_in_flight, conn_tuple_t, kafka_transaction_t, 0)

/* This map used for flush complete HTTP batches to userspace */
BPF_PERF_EVENT_ARRAY_MAP(kafka_batch_events, __u32, 0)
BPF_PERF_EVENT_ARRAY_MAP(kafka_events, __u32, 0)

/*
  This map stores finished KAFKA transactions in batches so they can be consumed by userspace
  Size is set dynamically during runtime and must be equal to CPUs*KAFKA_BATCH_PAGES
 */
BPF_HASH_MAP(kafka_batches, kafka_batch_key_t, kafka_batch_t, 0)

/* This map holds one entry per CPU storing state associated to current kafka batch*/
BPF_PERCPU_ARRAY_MAP(kafka_batch_state, __u32, kafka_batch_state_t, 1)
//
//BPF_LRU_MAP(ssl_sock_by_ctx, void *, ssl_sock_t, 1)
//
//BPF_LRU_MAP(ssl_read_args, u64, ssl_read_args_t, 1024)
//
//BPF_LRU_MAP(ssl_read_ex_args, u64, ssl_read_ex_args_t, 1024)
//
//BPF_LRU_MAP(ssl_write_args, u64, ssl_write_args_t, 1024)
//
//BPF_LRU_MAP(ssl_write_ex_args, u64, ssl_write_ex_args_t, 1024)
//
//BPF_LRU_MAP(bio_new_socket_args, __u64, __u32, 1024)
//
//BPF_LRU_MAP(fd_by_ssl_bio, __u32, void *, 1024)
//
//BPF_LRU_MAP(ssl_ctx_by_pid_tgid, __u64, void *, 1024)
//
//BPF_LRU_MAP(open_at_args, __u64, lib_path_t, 1024)
//
///* Map used to store the sub program actually used by the socket filter.
// * This is done to avoid memory limitation when attaching a filter to
// * a socket.
// * See: https://datadoghq.atlassian.net/wiki/spaces/NET/pages/2326855913/HTTP#Program-size-limit-for-socket-filters */
BPF_PROG_ARRAY(kafka_progs, 1)
//
///* This map used for notifying userspace of a shared library being loaded */
//BPF_PERF_EVENT_ARRAY_MAP(shared_libraries, __u32, 0)
//
#endif
