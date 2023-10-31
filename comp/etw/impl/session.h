#undef _WIN32_WINNT
#define _WIN32_WINNT _WIN32_WINNT_WINBLUE // Windows 8.1

#include <windows.h>
#include <evntcons.h>
#include <tdh.h>
#include <inttypes.h>

extern void etwCallbackC(PEVENT_RECORD);

static void WINAPI RecordEventCallback(PEVENT_RECORD event)
{
    etwCallbackC(event);
}

static TRACEHANDLE StartTracing(LPWSTR name, uintptr_t context)
{
    EVENT_TRACE_LOGFILEW trace = {0};
    trace.LoggerName = name;
    trace.Context = (void*)context;
    trace.ProcessTraceMode = PROCESS_TRACE_MODE_REAL_TIME | PROCESS_TRACE_MODE_EVENT_RECORD;
    trace.EventRecordCallback = RecordEventCallback;

    return OpenTraceW(&trace);
}
