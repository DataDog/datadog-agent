// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

#include "cpu_windows.h"

static inline void getCacheSize(SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX* ptr, CPU_INFO* cpuInfo) {
    switch (ptr->Cache.Level) {
        case 1:
            cpuInfo->l1CacheSize += ptr->Cache.CacheSize;
            break;
        case 2:
            cpuInfo->l2CacheSize += ptr->Cache.CacheSize;
            break;
        case 3:
            cpuInfo->l3CacheSize += ptr->Cache.CacheSize;
            break;
        default:
            // ignore, but this should not happen
            break;
    }
}

// computeCoresAndProcessors gets CPU information using Windows API
int computeCoresAndProcessors(CPU_INFO* cpuInfo) {
    DWORD buflen = 0;
    SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX* buffer = NULL;
    BOOL done = FALSE;
    DWORD returnLength = 0;
    DWORD byteOffset = 0;

    // Initialize the struct
    memset(cpuInfo, 0, sizeof(CPU_INFO));

    // First call to get required buffer size
    // NOTE: The following call always return failure because the intention is to
    // get the buffer size so we provide NULL as the buffer. The error we need to
    // check is ERROR_INSUFFICIENT_BUFFER. Any other error will trigger a return.
    BOOL ret = GetLogicalProcessorInformationEx(RelationAll, NULL, &buflen);
    if (FALSE != ret) {
        // this should not happen due to no buffer suppliedin the call
        return ERROR_INVALID_FUNCTION;
    }
    DWORD err = GetLastError();
    if (err != ERROR_INSUFFICIENT_BUFFER) {
        return err;
    }

    buffer = (SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX*)malloc(buflen);
    if (buffer == NULL) {
        return ERROR_OUTOFMEMORY;
    }

    if (!GetLogicalProcessorInformationEx(RelationAll, buffer, &buflen)) {
        free(buffer);
        return GetLastError();
    }

    // Walk through the buffer
    byteOffset = 0;
    while (byteOffset < buflen) {
        SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX* ptr =
            (SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX*)((char*)buffer + byteOffset);

        switch (ptr->Relationship) {
            case RelationProcessorCore:
                cpuInfo->corecount++;
                // Count logical processors in this core
                for (WORD i = 0; i < ptr->Processor.GroupCount; i++) {
                    GROUP_AFFINITY* groupAffinity = &ptr->Processor.GroupMask[i];
                    ULONG64 mask = groupAffinity->Mask;
                    cpuInfo->logicalcount += (int)__popcnt64(mask);
                }
                break;

            case RelationNumaNode:
                cpuInfo->numaNodeCount++;
                break;

            case RelationCache:
                getCacheSize(ptr, cpuInfo);
                break;

            case RelationProcessorPackage:
                cpuInfo->pkgcount++;
                break;

            case RelationGroup:
                cpuInfo->relationGroups = ptr->Group.MaximumGroupCount;
                for (WORD i = 0; i < ptr->Group.ActiveGroupCount; i++) {
                    cpuInfo->maxProcsInGroups += ptr->Group.GroupInfo[i].MaximumProcessorCount;
                    cpuInfo->activeProcsInGroups += ptr->Group.GroupInfo[i].ActiveProcessorCount;
                }
                break;

            default:
                // NOTE: ignore other relationships
                break;
        }

        byteOffset += ptr->Size;
    }

    free(buffer);
    return 0;
}

// getSystemInfo: gets system information using Windows API
int getSystemInfo(SYSTEM_INFO* sysInfo) {
    memset(sysInfo, 0, sizeof(SYSTEM_INFO));
    GetSystemInfo(sysInfo);
    return 0;
} 