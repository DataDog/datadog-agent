#ifndef __KAFKA_PARSING_MAPS_H
#define __KAFKA_PARSING_MAPS_H

BPF_PERCPU_ARRAY_MAP(kafka_heap, kafka_transaction_t, 1)

/*
 * This BPF map is utilized for kernel-space telemetry.
 * Only key 0 is utilized, and its corresponding value is a Kafka telemetry object.
 */
BPF_ARRAY_MAP(kafka_telemetry, kafka_telemetry_t, 1)

#endif
