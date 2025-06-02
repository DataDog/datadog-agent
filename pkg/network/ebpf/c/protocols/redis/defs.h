#ifndef __REDIS_DEFS_H
#define __REDIS_DEFS_H

#define REDIS_MIN_FRAME_LENGTH 3

#define REDIS_CMD_GET "GET"
#define REDIS_CMD_SET "SET"
#define RESP_ARRAY_PREFIX '*'
#define RESP_BULK_PREFIX '$'
#define RESP_SIMPLE_STRING_PREFIX '+'
#define RESP_ERROR_PREFIX '-'
#define RESP_FIELD_TERMINATOR_LEN 2 // CRLF terminator: \r\n
#define METHOD_LEN 3 // We only support GET and SET for now, both with length 3.
#define MAX_DIGITS_KEY_LEN_PREFIX 3 // Since we clamp key length to 128, when reading key length prefix, we only need to read up to 3 digits.
#define MAX_KEY_LEN 128
#define MIN_PARAM_COUNT 2 // GET command has 2 parameters
#define MAX_PARAM_COUNT 5 // SET command has 3-5 parameters
#define MAX_READABLE_KEY_LEN 999 // Since we read up to 3 digits of key length, the maximum readable length is 999.
#define READ_KEY_CHUNK_SIZE 16 // Read keys in chunks of length 16
#define RESP_TERMINATOR_1 '\r'
#define RESP_TERMINATOR_2 '\n'
#endif
