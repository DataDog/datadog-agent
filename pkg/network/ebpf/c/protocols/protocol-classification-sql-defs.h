#ifndef __PROTOCOL_CLASSIFICATION_SQL_DEFS_H
#define __PROTOCOL_CLASSIFICATION_SQL_DEFS_H

#include "bpf_builtins.h"

#define SQL_ABORT "ABORT"
#define SQL_ALTER "ALTER"
#define SQL_ANALYZE "ANALYZE"
#define SQL_BEGIN "BEGIN"
#define SQL_CALL "CALL"
#define SQL_CHECKPOINT "CHECKPOINT"
#define SQL_CLOSE "CLOSE"
#define SQL_CLUSTER "CLUSTER"
#define SQL_COMMENT "COMMENT"
#define SQL_COMMIT "COMMIT"
#define SQL_COPY "COPY"
#define SQL_CREATE "CREATE"
#define SQL_DEALLOCATE "DEALLOCATE"
#define SQL_DECLARE "DECLARE"
#define SQL_DELETE "DELETE"
#define SQL_DISCARD "DISCARD"
#define SQL_DO "DO"
#define SQL_DROP "DROP"
#define SQL_END "END"
#define SQL_EXECUTE "EXECUTE"
#define SQL_EXPLAIN "EXPLAIN"
#define SQL_FETCH "FETCH"
#define SQL_GRANT "GRANT"
#define SQL_IMPORT_FOREIGN_SCHEMA "IMPORT FOREIGN SCHEMA"
#define SQL_INSERT "INSERT"
#define SQL_LISTEN "LISTEN"
#define SQL_LOAD "LOAD"
#define SQL_LOCK "LOCK"
#define SQL_MERGE "MERGE"
#define SQL_MOVE "MOVE"
#define SQL_NOTIFY "NOTIFY"
#define SQL_PREPARE "PREPARE"
#define SQL_REASSIGN_OWNED "REASSIGN OWNED"
#define SQL_REFRESH_MATERIALIZED_VIEW "REFRESH MATERIALIZED VIEW"
#define SQL_REINDEX "REINDEX"
#define SQL_RELEASE_SAVEPOINT "RELEASE SAVEPOINT"
#define SQL_RESET "RESET"
#define SQL_REVOKE "REVOKE"
#define SQL_ROLLBACK "ROLLBACK"
#define SQL_SAVEPOINT "SAVEPOINT"
#define SQL_SECURITY_LABEL "SECURITY_LABEL"
#define SQL_SELECT "SELECT"
#define SQL_SET "SET"
#define SQL_SHOW "SHOW"
#define SQL_START_TRANSACTION "START_TRANSACTION"
#define SQL_TRUNCATE "TRUNCATE"
#define SQL_UNLISTEN "UNLISTEN"
#define SQL_UPDATE "UPDATE"
#define SQL_VACUUM "VACUUM"
#define SQL_VALUES "VALUES"

#define check_command(Buf, Command, Size) ( \
    (Size >= (sizeof(Command) - 1))         \
    && Buf[0] == Command[0]                 \
    && !bpf_memcmp((Buf + 1), &(Command) + 1, sizeof(Command) - 2))

static __always_inline bool is_sql_command(const char *buf, __u32 buf_size) {
    return check_command(buf, SQL_ALTER, buf_size)
        || check_command(buf, SQL_CREATE, buf_size)
        || check_command(buf, SQL_DELETE, buf_size)
        || check_command(buf, SQL_DROP, buf_size)
        || check_command(buf, SQL_INSERT, buf_size)
        || check_command(buf, SQL_SELECT, buf_size)
        || check_command(buf, SQL_UPDATE, buf_size);
}

#endif /*__PROTOCOL_CLASSIFICATION_SQL_DEFS_H*/
