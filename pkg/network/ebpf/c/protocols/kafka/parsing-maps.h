#ifndef __KAFKA_PARSING_MAPS_H
#define __KAFKA_PARSING_MAPS_H

BPF_PERCPU_ARRAY_MAP(kafka_heap, kafka_transaction_t, 1)
/*
    This map help us to avoid processing the same traffic twice.
    It holds the last tcp sequence number for each connection.
   */
BPF_HASH_MAP(kafka_last_tcp_seq_per_connection, conn_tuple_t, __u32, 0)

#endif
