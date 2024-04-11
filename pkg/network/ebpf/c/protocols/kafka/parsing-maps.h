#ifndef __KAFKA_PARSING_MAPS_H
#define __KAFKA_PARSING_MAPS_H

BPF_PERCPU_ARRAY_MAP(kafka_heap, kafka_transaction_t, 1)

#endif
