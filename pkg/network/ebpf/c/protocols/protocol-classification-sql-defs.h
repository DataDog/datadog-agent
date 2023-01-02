#ifndef __PROTOCOL_CLASSIFICATION_SQL_DEFS_H
#define __PROTOCOL_CLASSIFICATION_SQL_DEFS_H

#include "bpf_builtins.h"

#define SQL_COMMAND_MAX_SIZE 6

#define SQL_ALTER "ALTER"
#define SQL_CREATE "CREATE"
#define SQL_DELETE "DELETE"
#define SQL_DROP "DROP"
#define SQL_INSERT "INSERT"
#define SQL_SELECT "SELECT"
#define SQL_UPDATE "UPDATE"

// Check that we can read the amount of memory we want, then to the comparison.
// Note: we use `sizeof(command) - 1` to *not* compare with the null-terminator of
// the strings.
#define check_command(buf, command, buf_size) ( \
    ((sizeof(command) - 1) <= buf_size)         \
    && !bpf_memcmp((buf), &(command), sizeof(command) - 1))

static __always_inline bool is_sql_command(const char *buf, __u32 buf_size) {
    char tmp[SQL_COMMAND_MAX_SIZE];

#pragma unroll (SQL_COMMAND_MAX_SIZE)
    for (int i = 0; i < SQL_COMMAND_MAX_SIZE; i++) {
        if ('a' <= buf[i] && buf[i] <= 'z') {
            tmp[i] = buf[i] - 'a' +'A';
        } else {
            tmp[i] = buf[i];
        }
    }

    return check_command(tmp, SQL_ALTER, buf_size)
        || check_command(tmp, SQL_CREATE, buf_size)
        || check_command(tmp, SQL_DELETE, buf_size)
        || check_command(tmp, SQL_DROP, buf_size)
        || check_command(tmp, SQL_INSERT, buf_size)
        || check_command(tmp, SQL_SELECT, buf_size)
        || check_command(tmp, SQL_UPDATE, buf_size);
}

#endif /*__PROTOCOL_CLASSIFICATION_SQL_DEFS_H*/
