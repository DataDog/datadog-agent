#ifndef __WINUTIL_ETW_PROPERTIES_H__
#define __WINUTIL_ETW_PROPERTIES_H__

#include "etw.h"

LPCWSTR DDGetPropertyName(PTRACE_EVENT_INFO info, int i);
ULONG DDGetArraySize(PEVENT_RECORD event, PTRACE_EVENT_INFO info, int i, UINT32* count);
ULONG DDGetPropertyLength(PEVENT_RECORD event, PTRACE_EVENT_INFO info, int i, UINT32* propertyLength);
USHORT DDGetInType(PTRACE_EVENT_INFO info, int i);
USHORT DDGetOutType(PTRACE_EVENT_INFO info, int i);
LPWSTR DDGetMapName(PTRACE_EVENT_INFO info, int i);
BOOL DDPropertyIsStruct(PTRACE_EVENT_INFO info, int i);
BOOL DDPropertyIsArray(PTRACE_EVENT_INFO info, int i);
int DDGetStructStartIndex(PTRACE_EVENT_INFO info, int i);
int DDGetStructLastIndex(PTRACE_EVENT_INFO info, int i);
PEVENT_MAP_INFO DDGetMapInfo(PEVENT_RECORD event, PTRACE_EVENT_INFO info, int i);
void DDFreeMapInfo(PEVENT_MAP_INFO mapInfo);
ULONG DDTdhFormatProperty(PTRACE_EVENT_INFO info, PEVENT_MAP_INFO mapInfo, ULONG pointerSize, ULONG inType, ULONG outType,
    ULONG propertyLength, ULONG userDataLength, ULONG_PTR userData, PINT formattedDataSize, PUCHAR formattedData, PUSHORT userDataConsumed);
ULONG DDGetPropertyByName(PEVENT_RECORD event, LPCWSTR propertyName, PBYTE buffer, ULONG bufferSize, PULONG bytesWritten);

#endif
