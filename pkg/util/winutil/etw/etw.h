#ifndef __WINUTIL_ETW_H__
#define __WINUTIL_ETW_H__

#undef _WIN32_WINNT
#define _WIN32_WINNT _WIN32_WINNT_WINBLUE

#include <windows.h>
#include <evntcons.h>
#include <tdh.h>

// DDOpenTraceFromFile opens an ETL file for reading and returns a trace handle.
// Uses LogFileName (not LoggerName) for file-based consumption.
// callbackContext is passed to the event callback via UserContext.
// On failure (returns INVALID_PROCESSTRACE_HANDLE), *errCode receives GetLastError().
TRACEHANDLE DDOpenTraceFromFile(LPCWSTR logFileName, uintptr_t callbackContext, PULONG errCode);

// DDProcessETLFile processes events from an open trace handle.
// Blocks until all events are processed or an error occurs.
ULONG DDProcessETLFile(TRACEHANDLE traceHandle);

// DDStopETWSession stops an ETW trace session by name.
// Uses ControlTraceW with EVENT_TRACE_CONTROL_STOP.
ULONG DDStopETWSession(LPCWSTR sessionName);

// DDGetEventContext returns the UserContext from EVENT_RECORD (set via EVENT_TRACE_LOGFILEW.Context).
uintptr_t DDGetEventContext(PEVENT_RECORD eventRecord);

#endif
