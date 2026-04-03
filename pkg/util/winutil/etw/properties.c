#include "properties.h"
#include <stdlib.h>
#include <string.h>
#include <in6addr.h>

#define PropertyParamCount 0x0001
#define PropertyParamLength 0x0002
#define PropertyStruct 0x0004
#define TDH_INTYPE_BINARY 14
#define TDH_OUTTYPE_IPV6 24
#ifndef ERROR_EVT_INVALID_EVENT_DATA
#define ERROR_EVT_INVALID_EVENT_DATA 0x3ab5
#endif

/* MinGW-w64 tdh.h may not declare TdhFormatProperty */
ULONG WINAPI TdhFormatProperty(
    PTRACE_EVENT_INFO EventRecord,
    PEVENT_MAP_INFO MapInfo,
    ULONG PointerSize,
    USHORT PropertyInType,
    USHORT PropertyOutType,
    USHORT PropertyLength,
    USHORT UserDataLength,
    PBYTE UserData,
    PULONG BufferSize,
    PWCHAR Buffer,
    PUSHORT UserDataConsumed);

static int getLengthFromProperty(PEVENT_RECORD event, PROPERTY_DATA_DESCRIPTOR* dataDescriptor, UINT32* length) {
    DWORD propertySize = 0;
    ULONG status = TdhGetPropertySize(event, 0, NULL, 1, dataDescriptor, &propertySize);
    if (status != ERROR_SUCCESS) {
        return (int)status;
    }
    status = TdhGetProperty(event, 0, NULL, 1, dataDescriptor, propertySize, (PBYTE)length);
    return (int)status;
}

LPCWSTR DDGetPropertyName(PTRACE_EVENT_INFO info, int i) {
    return (LPCWSTR)((PBYTE)(info) + info->EventPropertyInfoArray[i].NameOffset);
}

ULONG DDGetArraySize(PEVENT_RECORD event, PTRACE_EVENT_INFO info, int i, UINT32* count) {
    if ((info->EventPropertyInfoArray[i].Flags & PropertyParamCount) == PropertyParamCount) {
        PROPERTY_DATA_DESCRIPTOR dataDescriptor = {0};
        dataDescriptor.PropertyName = (ULONGLONG)((PBYTE)(info) + info->EventPropertyInfoArray[info->EventPropertyInfoArray[i].countPropertyIndex].NameOffset);
        dataDescriptor.ArrayIndex = ULONG_MAX;
        return (ULONG)getLengthFromProperty(event, &dataDescriptor, count);
    }
    *count = info->EventPropertyInfoArray[i].count;
    return ERROR_SUCCESS;
}

ULONG DDGetPropertyLength(PEVENT_RECORD event, PTRACE_EVENT_INFO info, int i, UINT32* propertyLength) {
    if ((info->EventPropertyInfoArray[i].Flags & PropertyParamLength) == PropertyParamLength) {
        PROPERTY_DATA_DESCRIPTOR dataDescriptor = {0};
        dataDescriptor.PropertyName = (ULONGLONG)((PBYTE)(info) + info->EventPropertyInfoArray[info->EventPropertyInfoArray[i].lengthPropertyIndex].NameOffset);
        dataDescriptor.ArrayIndex = ULONG_MAX;
        getLengthFromProperty(event, &dataDescriptor, propertyLength);
        return ERROR_SUCCESS;
    }
    if (info->EventPropertyInfoArray[i].nonStructType.InType == TDH_INTYPE_BINARY &&
        info->EventPropertyInfoArray[i].nonStructType.OutType == TDH_OUTTYPE_IPV6) {
        *propertyLength = sizeof(IN6_ADDR);
        return ERROR_SUCCESS;
    }
    *propertyLength = info->EventPropertyInfoArray[i].length;
    return ERROR_SUCCESS;
}

USHORT DDGetInType(PTRACE_EVENT_INFO info, int i) {
    return info->EventPropertyInfoArray[i].nonStructType.InType;
}

USHORT DDGetOutType(PTRACE_EVENT_INFO info, int i) {
    return info->EventPropertyInfoArray[i].nonStructType.OutType;
}

LPWSTR DDGetMapName(PTRACE_EVENT_INFO info, int i) {
    return (LPWSTR)((PBYTE)(info) + info->EventPropertyInfoArray[i].nonStructType.MapNameOffset);
}

BOOL DDPropertyIsStruct(PTRACE_EVENT_INFO info, int i) {
    return (info->EventPropertyInfoArray[i].Flags & PropertyStruct) == PropertyStruct;
}

BOOL DDPropertyIsArray(PTRACE_EVENT_INFO info, int i) {
    return ((info->EventPropertyInfoArray[i].Flags & PropertyParamCount) == PropertyParamCount) ||
           (info->EventPropertyInfoArray[i].count > 1);
}

int DDGetStructStartIndex(PTRACE_EVENT_INFO info, int i) {
    return info->EventPropertyInfoArray[i].structType.StructStartIndex;
}

int DDGetStructLastIndex(PTRACE_EVENT_INFO info, int i) {
    return info->EventPropertyInfoArray[i].structType.StructStartIndex +
           info->EventPropertyInfoArray[i].structType.NumOfStructMembers;
}

PEVENT_MAP_INFO DDGetMapInfo(PEVENT_RECORD event, PTRACE_EVENT_INFO info, int i) {
    LPWSTR mapName = DDGetMapName(info, i);
    ULONG mapSize = 0;
    ULONG ret = TdhGetEventMapInformation(event, mapName, NULL, &mapSize);
    if (ret == ERROR_NOT_FOUND) {
        return NULL;
    }
    if (ret != ERROR_INSUFFICIENT_BUFFER) {
        return NULL;
    }
    PEVENT_MAP_INFO mapInfo = (PEVENT_MAP_INFO)malloc(mapSize);
    if (!mapInfo) {
        return NULL;
    }
    ret = TdhGetEventMapInformation(event, mapName, mapInfo, &mapSize);
    if (ret != ERROR_SUCCESS) {
        free(mapInfo);
        return NULL;
    }
    return mapInfo;
}

void DDFreeMapInfo(PEVENT_MAP_INFO mapInfo) {
    if (mapInfo != NULL) {
        free(mapInfo);
    }
}

ULONG DDGetPropertyByName(PEVENT_RECORD event, LPCWSTR propertyName, PBYTE buffer, ULONG bufferSize, PULONG bytesWritten) {
    PROPERTY_DATA_DESCRIPTOR descriptor = {0};
    descriptor.PropertyName = (ULONGLONG)propertyName;
    descriptor.ArrayIndex = ULONG_MAX;

    ULONG propertySize = 0;
    ULONG status = TdhGetPropertySize(event, 0, NULL, 1, &descriptor, &propertySize);
    if (status != ERROR_SUCCESS) {
        return status;
    }
    if (propertySize > bufferSize) {
        *bytesWritten = propertySize;
        return ERROR_INSUFFICIENT_BUFFER;
    }
    *bytesWritten = propertySize;
    return TdhGetProperty(event, 0, NULL, 1, &descriptor, propertySize, buffer);
}

ULONG DDTdhFormatProperty(PTRACE_EVENT_INFO info, PEVENT_MAP_INFO mapInfo, ULONG pointerSize, ULONG inType, ULONG outType,
    ULONG propertyLength, ULONG userDataLength, ULONG_PTR userData, PINT formattedDataSize, PUCHAR formattedData, PUSHORT userDataConsumed) {
    return TdhFormatProperty(
        info,
        mapInfo,
        (ULONG)pointerSize,
        (USHORT)inType,
        (USHORT)outType,
        (USHORT)propertyLength,
        (USHORT)userDataLength,
        (PBYTE)userData,
        (PULONG)formattedDataSize,
        (PWCHAR)formattedData,
        userDataConsumed);
}
