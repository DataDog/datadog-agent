#ifndef __KAFKA_MAPS_H
#define __KAFKA_MAPS_H

#include "tracer.h"
#include "bpf_helpers.h"
#include "kafka-types.h"
#include "map-defs.h"

/*
    This map help us to avoid processing the same traffic twice.
    It holds the last tcp sequence number for each connection.
   */
BPF_HASH_MAP(kafka_last_tcp_seq_per_connection, conn_tuple_t, __u32, 0)

BPF_PERCPU_ARRAY_MAP(kafka_heap, __u32, kafka_transaction_t, 1)

///* Map used to store the sub program actually used by the socket filter.
// * This is done to avoid memory limitation when attaching a filter to
// * a socket.
// * See: https://datadoghq.atlassian.net/wiki/spaces/NET/pages/2326855913/HTTP#Program-size-limit-for-socket-filters */
BPF_PROG_ARRAY(kafka_progs, 1)

#endif
