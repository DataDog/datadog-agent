#include "etw.h"
#include <stdlib.h>
#include <string.h>

extern void ddEtwEventCallback(PEVENT_RECORD eventRecord);

static void WINAPI eventRecordCallback(PEVENT_RECORD eventRecord) {
    ddEtwEventCallback(eventRecord);
}

TRACEHANDLE DDOpenTraceFromFile(LPCWSTR logFileName, uintptr_t callbackContext, PULONG errCode) {
    EVENT_TRACE_LOGFILEW trace = {0};
    trace.LogFileName = (LPWSTR)logFileName;
    trace.LoggerName = NULL;
    trace.ProcessTraceMode = PROCESS_TRACE_MODE_EVENT_RECORD;
    trace.EventRecordCallback = eventRecordCallback;
    trace.Context = (PVOID)callbackContext;

    TRACEHANDLE h = OpenTraceW(&trace);
    if (h == INVALID_PROCESSTRACE_HANDLE) {
        *errCode = GetLastError();
    } else {
        *errCode = ERROR_SUCCESS;
    }
    return h;
}

ULONG DDProcessETLFile(TRACEHANDLE traceHandle) {
    return ProcessTrace(&traceHandle, 1, NULL, NULL);
}

uintptr_t DDGetEventContext(PEVENT_RECORD eventRecord) {
    return (uintptr_t)eventRecord->UserContext;
}

ULONG DDStopETWSession(LPCWSTR sessionName) {
    const DWORD maxLogFileName = 1024;
    DWORD sessionNameLen = (DWORD)(wcslen(sessionName) + 1) * sizeof(WCHAR);
    DWORD bufSize = sizeof(EVENT_TRACE_PROPERTIES) + sessionNameLen + maxLogFileName;
    PEVENT_TRACE_PROPERTIES pProperties = (PEVENT_TRACE_PROPERTIES)malloc(bufSize);
    if (!pProperties) {
        return ERROR_OUTOFMEMORY;
    }
    memset(pProperties, 0, bufSize);
    pProperties->Wnode.BufferSize = bufSize;

    ULONG ret = ControlTraceW(0, sessionName, pProperties, EVENT_TRACE_CONTROL_STOP);
    free(pProperties);
    return ret;
}
