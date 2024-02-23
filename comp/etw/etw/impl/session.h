#ifndef __SESSION_H__
#define __SESSION_H__

#undef _WIN32_WINNT
#define _WIN32_WINNT _WIN32_WINNT_WINBLUE // Windows 8.1

#include <windows.h>
#include <evntcons.h>
#include <tdh.h>
#include <inttypes.h>

#define EVENT_FILTER_TYPE_EVENT_ID          (0x80000200)
#define EVENT_FILTER_TYPE_PID               (0x80000004)

ULONG DDEnableTrace(
    TRACEHANDLE TraceHandle,
    LPCGUID     ProviderId,
    ULONG       ControlCode,
    UCHAR       Level,
    ULONGLONG   MatchAnyKeyword,
    ULONGLONG   MatchAllKeyword,
    ULONG       Timeout,
    ULONG*      PIDs,
    ULONG       PIDCount
);
TRACEHANDLE DDStartTracing(LPWSTR name, uintptr_t context);

#endif
