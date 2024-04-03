#ifndef __KAFKA_PARSING_MAPS_H
#define __KAFKA_PARSING_MAPS_H

BPF_PERCPU_ARRAY_MAP(kafka_heap, kafka_info_t, 1)

BPF_HASH_MAP(kafka_in_flight, kafka_transaction_key_t, kafka_transaction_t, 0)
BPF_HASH_MAP(kafka_response, conn_tuple_t, kafka_response_context_t, 0)
BPF_HASH_MAP(kafka_tcp_seq, conn_tuple_t, u32, 0)

#endif
