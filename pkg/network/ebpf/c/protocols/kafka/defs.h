#ifndef __KAFKA_DEFS_H
#define __KAFKA_DEFS_H

// Reference: https://kafka.apache.org/protocol.html#protocol_messages
#define KAFKA_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION 12
#define KAFKA_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION 8

#define KAFKA_MIN_LENGTH (sizeof(kafka_header_t))
#define CLIENT_ID_SIZE_TO_VALIDATE 30
#define TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE 48 // 16 * 3. Must be a factor of 16, otherwise a verifier issue can pop in kernel 4.14.
#define TOPIC_NAME_MAX_ALLOWED_SIZE 255

#define TOPIC_NAME_MAX_STRING_SIZE 80

// The number of varint bytes required to support the specified values.
// 127
#define VARINT_BYTES_0000007f   1
// 16383
#define VARINT_BYTES_00003fff   2
// 2097151
#define VARINT_BYTES_001fffff   3
// 268435455
#define VARINT_BYTES_0fffffff   4

#define VARINT_BYTES_NUM_TOPICS VARINT_BYTES_001fffff
#define VARINT_BYTES_TOPIC_NAME_SIZE VARINT_BYTES_001fffff
#define VARINT_BYTES_NUM_PARTITIONS VARINT_BYTES_001fffff
#define VARINT_BYTES_NUM_ABORTED_TRANSACTIONS VARINT_BYTES_001fffff
#define VARINT_BYTES_RECORD_BATCHES_NUM_BYTES VARINT_BYTES_001fffff

#define KAFKA_RESPONSE_PARSER_MAX_ITERATIONS 10

// We do not have a way to validate the size of the aborted transactions list
// and if we misinterpret a packet we could end up waiting for a large number
// of bytes for the list to end. This limit is used as a heuristic to prevent
// that. This could be removed/revisited after the TCP stream handling to
// prevent seeing out-of-order packets has seen more testing.
#define KAFKA_MAX_ABORTED_TRANSACTIONS 10000

// This controls the number of Kafka transactions read from userspace at a time
#define KAFKA_BATCH_SIZE 28

// The amount of buckets we have for the kafka topic name length telemetry.
#define KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS 10

// The size of each kafka telemetry topic name bucket
#define KAFKA_TELEMETRY_TOPIC_NAME_BUCKET_SIZE 10

#endif
