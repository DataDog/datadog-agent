// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#ifndef DD_GPU_AGX_DARWIN_H
#define DD_GPU_AGX_DARWIN_H

#include <stdbool.h>
#include <stdint.h>

#define DD_MAX_AGX_DEVICES 16
#define DD_AGX_MODEL_LENGTH 256

typedef struct {
    char model[DD_AGX_MODEL_LENGTH];
    bool hasModel;
    int64_t coreCount;
    bool hasCoreCount;
    double utilization;
    bool hasUtilization;
    int64_t allocatedSystemMemory;
    bool hasAllocatedSystemMemory;
    int64_t inUseSystemMemory;
    bool hasInUseSystemMemory;
} DDAGXDeviceInfo;

typedef struct {
    uint32_t count;
    uint32_t propertyReadErrors;
    bool truncated;
    int32_t error;
} DDAGXCollectionResult;

DDAGXCollectionResult dd_collect_agx_devices(DDAGXDeviceInfo *devices, uint32_t capacity);

#endif
