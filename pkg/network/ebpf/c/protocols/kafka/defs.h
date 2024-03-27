#ifndef __KAFKA_DEFS_H
#define __KAFKA_DEFS_H

// Reference: https://kafka.apache.org/protocol.html#protocol_messages
#define KAFKA_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION 11
#define KAFKA_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION 8

#define KAFKA_MIN_LENGTH (sizeof(kafka_header_t))
#define CLIENT_ID_SIZE_TO_VALIDATE 30
#define TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE 48 // 16 * 3. Must be a factor of 16, otherwise a verifier issue can pop in kernel 4.14.
#define TOPIC_NAME_MAX_ALLOWED_SIZE 255

#define TOPIC_NAME_MAX_STRING_SIZE 80

// This controls the number of Kafka transactions read from userspace at a time
#define KAFKA_BATCH_SIZE 28

#endif
