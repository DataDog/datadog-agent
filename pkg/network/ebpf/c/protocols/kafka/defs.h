#ifndef __KAFKA_DEFS_H
#define __KAFKA_DEFS_H

// Reference: https://kafka.apache.org/protocol.html#protocol_messages
#define KAFKA_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION 11
#define KAFKA_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION 8

#define CLIENT_ID_SIZE_TO_VALIDATE 30
#define TOPIC_NAME_MAX_STRING_SIZE 80

// Every kafka message encodes starts with:
//  * 4 bytes for the size of the payload
//  * 2 bytes for api key
//  * 2 bytes for api version
//  * 4 bytes for correlation id
// Reference: https://kafka.apache.org/protocol.html#protocol_messages
#define KAFKA_MIN_SIZE 12

//// This determines the size of the payload fragment that is captured for each HTTP request
#define KAFKA_BUFFER_SIZE (8 * 40) // 320

// This is needed to reduce code size on multiple copy optimizations that were made in
// the http eBPF program.
_Static_assert((KAFKA_BUFFER_SIZE % 8) == 0, "KAFKA_BUFFER_SIZE must be a multiple of 8.");

//// This controls the number of Kafka transactions read from userspace at a time
#define KAFKA_BATCH_SIZE 15

#define KAFKA_PROG 0

#endif
