// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

#ifndef CPU_WINDOWS_H
#define CPU_WINDOWS_H

#include <stddef.h>
#include <windows.h>
#include <intrin.h>  // for __popcnt64

// CPU_INFO mirrors the Go struct
typedef struct {
    int corecount;
    int logicalcount;
    int pkgcount;
    int numaNodeCount;
    int relationGroups;
    int maxProcsInGroups;
    int activeProcsInGroups;
    unsigned long long l1CacheSize;
    unsigned long long l2CacheSize;
    unsigned long long l3CacheSize;
} CPU_INFO;

// computeCoresAndProcessors gets CPU information using Windows API
int computeCoresAndProcessors(CPU_INFO* cpuInfo);

// getSystemInfo gets system information using Windows API
int getSystemInfo(SYSTEM_INFO* sysInfo);

#endif // CPU_WINDOWS_H