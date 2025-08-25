
#ifndef DD_CRASHDUMP_H
#define DD_CRASHDUMP_H

#include <windows.h>
#include <winerror.h>
#include <dbghelp.h>
#include <dbgeng.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef enum _readCrashDumpErrors {
    RCD_NONE = 0,
    RCD_DEBUG_CREATE_FAILED = 1,
    RCD_QUERY_INTERFACE_FAILED = 2,
    RCD_SET_OUTPUT_CALLBACKS_FAILED = 3,
    RCD_OPEN_DUMP_FILE_FAILED = 4,
    RCD_WAIT_FOR_EVENT_FAILED = 5,
    RCD_EXECUTE_FAILED = 6,
    RCD_INVALID_ARG = 7
} READ_CRASH_DUMP_ERROR;

typedef struct _bugCheckInfo {
    ULONG code;
    ULONG64 arg1;
    ULONG64 arg2;
    ULONG64 arg3;
    ULONG64 arg4;
} BUGCHECK_INFO;

READ_CRASH_DUMP_ERROR readCrashDump(char* fname, void* ctx, BUGCHECK_INFO* bugCheckInfo, long* extendedError);

#ifdef __cplusplus
} // close the extern "C"
#endif
#endif /* DD_CRASHDUMP_H */
