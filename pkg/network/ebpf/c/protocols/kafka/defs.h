#ifndef __KAFKA_DEFS_H
#define __KAFKA_DEFS_H

// The maximum request API version for fetch request is 13
// The maximum request API version for produce is 9
// So setting it to the maximum between the 2
// Reference: https://kafka.apache.org/protocol.html#protocol_messages
#define KAFKA_MAX_SUPPORTED_REQUEST_API_VERSION 13

#define KAFKA_MIN_LENGTH 14
#define CLIENT_ID_SIZE_TO_VALIDATE 20
#define TOPIC_NAME_MAX_STRING_SIZE (8 * 10)

#endif
