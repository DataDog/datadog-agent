#include "session.h"

// This constant defines the maximum number of filter types supported.
#define MAX_FILTER_SUPPORTED                4

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
    ULONG       PIDCount,
    USHORT*     enableFilterIDs,
    ULONG       enableFilterIDCount,
    USHORT*     disableFilterIDs,
    ULONG       disableFilterIDCount
)
{
    EVENT_FILTER_DESCRIPTOR eventFilterDescriptors[MAX_FILTER_SUPPORTED];

    ENABLE_TRACE_PARAMETERS enableParameters = { 0 };
    enableParameters.Version = ENABLE_TRACE_PARAMETERS_VERSION_2;
    enableParameters.EnableFilterDesc = &eventFilterDescriptors[0];
    enableParameters.FilterDescCount = 0;
    int eventFilterDescriptorIndex = 0;
    ULONG ret = 0;

    PEVENT_FILTER_EVENT_ID  enabledFilters = NULL;
    PEVENT_FILTER_EVENT_ID  disabledFilters = NULL;
    if (PIDCount > 0)
    {
        eventFilterDescriptors[eventFilterDescriptorIndex].Ptr  = (ULONGLONG)PIDs;
        eventFilterDescriptors[eventFilterDescriptorIndex].Size = (ULONG)(sizeof(PIDs[0]) * PIDCount);
        eventFilterDescriptors[eventFilterDescriptorIndex].Type = EVENT_FILTER_TYPE_PID;

        enableParameters.FilterDescCount++;
        eventFilterDescriptorIndex++;
    }

    if (enableFilterIDCount > 0)
    {
        ULONG size = sizeof(EVENT_FILTER_EVENT_ID) + (sizeof(enableFilterIDs[0]) * enableFilterIDCount);
        enabledFilters = (EVENT_FILTER_EVENT_ID*)malloc(size);

        enabledFilters->FilterIn = TRUE;
        enabledFilters->Count = enableFilterIDCount;
        for (int i =0; i < enableFilterIDCount; i++)
        {
            enabledFilters->Events[i] = enableFilterIDs[i];
        }
        eventFilterDescriptors[eventFilterDescriptorIndex].Ptr  = (ULONGLONG)enabledFilters;
        eventFilterDescriptors[eventFilterDescriptorIndex].Size = size;
        eventFilterDescriptors[eventFilterDescriptorIndex].Type = EVENT_FILTER_TYPE_EVENT_ID;

        enableParameters.FilterDescCount++;
        eventFilterDescriptorIndex++;
    }

    if (disableFilterIDCount > 0)
    {
        ULONG size = sizeof(EVENT_FILTER_EVENT_ID) + (sizeof(enableFilterIDs[0]) * disableFilterIDCount);
        disabledFilters = (EVENT_FILTER_EVENT_ID*)malloc(size);

        disabledFilters->FilterIn = FALSE;
        disabledFilters->Count = disableFilterIDCount;
        for (int i =0; i < disableFilterIDCount; i++)
        {
            disabledFilters->Events[i] = disableFilterIDs[i];
        }
        eventFilterDescriptors[eventFilterDescriptorIndex].Ptr  = (ULONGLONG)disabledFilters;
        eventFilterDescriptors[eventFilterDescriptorIndex].Size = size;
        eventFilterDescriptors[eventFilterDescriptorIndex].Type = EVENT_FILTER_TYPE_EVENT_ID;

        enableParameters.FilterDescCount++;
        eventFilterDescriptorIndex++;
    }

    ret = EnableTraceEx2(
        TraceHandle,
        ProviderId,
        ControlCode,
        Level,
        MatchAnyKeyword,
        MatchAllKeyword,
        Timeout,
        &enableParameters
    );

    if (enabledFilters != NULL)
    {
        free(enabledFilters);
    }
    if (disabledFilters != NULL)
    {
        free(disabledFilters);
    }
    return ret;
}
