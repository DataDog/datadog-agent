#ifndef __KAFKA_PARSING_MAPS_H
#define __KAFKA_PARSING_MAPS_H

BPF_PERCPU_ARRAY_MAP(kafka_heap, kafka_transaction_t, 1)

/* This map is used for telemetry in kernelspace
 * only key 0 is used
 * value is a Kafka telemetry object
 */
BPF_ARRAY_MAP(kafka_telemetry, kafka_telemetry_t, 1)

#endif
