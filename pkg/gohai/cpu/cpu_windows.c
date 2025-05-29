#include "cpu_windows.h"
#include <windows.h>
#include <stdio.h>

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
    GetLogicalProcessorInformationEx(RelationAll, NULL, &buflen);
    if (GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
        return GetLastError();
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
        SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX* ptr = (SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX*)((char*)buffer + byteOffset);

        switch (ptr->Relationship) {
            case RelationProcessorCore:
                cpuInfo->corecount++;
                // Count logical processors in this core
                for (WORD i = 0; i < ptr->Processor.GroupCount; i++) {
                    GROUP_AFFINITY* groupAffinity = &ptr->Processor.GroupMask[i];
                    ULONG64 mask = groupAffinity->Mask;
                    while (mask) {
                        if (mask & 1) {
                            cpuInfo->logicalcount++;
                        }
                        mask >>= 1;
                    }
                }
                break;

            case RelationNumaNode:
                cpuInfo->numaNodeCount++;
                break;

            case RelationCache:
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
                }
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
        }

        byteOffset += ptr->Size;
    }

    free(buffer);
    return 0;
} 