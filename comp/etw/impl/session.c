#include "session.h"

// This constant defines the maximum number of filter types supported.
#define MAX_FILTER_SUPPORTED                2

extern void ddEtwCallbackC(PEVENT_RECORD);

static void WINAPI RecordEventCallback(PEVENT_RECORD event)
{
    ddEtwCallbackC(event);
}

TRACEHANDLE DDStartTracing(LPWSTR name, uintptr_t context)
{
    EVENT_TRACE_LOGFILEW trace = {0};
    trace.LoggerName = name;
    trace.Context = (void*)context;
    trace.ProcessTraceMode = PROCESS_TRACE_MODE_REAL_TIME | PROCESS_TRACE_MODE_EVENT_RECORD;
    trace.EventRecordCallback = RecordEventCallback;

    return OpenTraceW(&trace);
}

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
)
{
    EVENT_FILTER_DESCRIPTOR eventFilterDescriptors[MAX_FILTER_SUPPORTED];

    ENABLE_TRACE_PARAMETERS enableParameters = { 0 };
    enableParameters.Version = ENABLE_TRACE_PARAMETERS_VERSION_2;
    enableParameters.EnableFilterDesc = &eventFilterDescriptors[0];
    enableParameters.FilterDescCount = 0;

    if (PIDCount > 0)
    {
        eventFilterDescriptors[0].Ptr  = (ULONGLONG)PIDs;
        eventFilterDescriptors[0].Size = (ULONG)(sizeof(PIDs[0]) * PIDCount);
        eventFilterDescriptors[0].Type = EVENT_FILTER_TYPE_PID;

        enableParameters.FilterDescCount++;
    }

    return EnableTraceEx2(
        TraceHandle,
        ProviderId,
        ControlCode,
        Level,
        MatchAnyKeyword,
        MatchAllKeyword,
        Timeout,
        &enableParameters
    );
}