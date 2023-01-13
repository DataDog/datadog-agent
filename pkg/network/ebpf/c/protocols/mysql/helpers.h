#ifndef __MYSQL_HELPERS_H
#define __MYSQL_HELPERS_H

#include "protocols/classification/common.h"
#include "protocols/mysql/defs.h"
#include "protocols/sql/helpers.h"

// Validates the given buffer is of the format <number><delimiter> where the number is up to 2 digits.
// The buffer cannot be just the delimiter. On error returns -1, on success the location of the next element.
static __always_inline int is_version_component_helper(const char *buf, char delimiter) {
    char current_char;
#pragma unroll MAX_VERSION_COMPONENT
    for (unsigned i = 0; i < MAX_VERSION_COMPONENT; i++) {
        current_char = buf[i];
        if ('0' <= current_char && current_char <= '9') {
            continue;
        }
        if (current_char == delimiter && i > 0) {
            return i+1;
        }
        // Any other character is not supported.
        break;
   }
   return -1;
}

// Checks if the given buffer is a null terminated string that represents a version of the format <major>.<minor>.<bugfix>
// where the major, minor and bugfix are numbers of max 2 digits each.
static __always_inline bool is_version(const char* buf, __u32 buf_size) {
    if (buf_size < MIN_VERSION_SIZE) {
        return false;
    }

    int major_pos_end = is_version_component_helper(buf, '.');
    if (major_pos_end == -1 || buf_size < major_pos_end+MAX_VERSION_COMPONENT) {
        return false;
    }

    int minor_pos_end = is_version_component_helper(buf+major_pos_end, '.');
    if (minor_pos_end == -1 || buf_size < minor_pos_end+major_pos_end+MAX_VERSION_COMPONENT) {
        return false;
    }
    return is_version_component_helper(buf+minor_pos_end+major_pos_end, '\0') != -1;
}

static __always_inline bool is_mysql(conn_tuple_t *tup, const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, MYSQL_MIN_LENGTH);

    mysql_hdr header = *((mysql_hdr *)buf);
    if (header.payload_length == 0) {
        return false;
    }

    switch (header.command_type) {
    case MYSQL_COMMAND_QUERY:
    case MYSQL_PREPARE_QUERY:
        return is_sql_command((char*)(buf+sizeof(mysql_hdr)), buf_size-sizeof(mysql_hdr));
    case MYSQL_SERVER_GREETING_V10:
    case MYSQL_SERVER_GREETING_V9:
        return is_version((char*)(buf+sizeof(mysql_hdr)), buf_size-sizeof(mysql_hdr));
    default:
        return false;
    }
}

#endif
