// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#import <CoreFoundation/CoreFoundation.h>
#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>
#import <string.h>

#import "agx_darwin.h"

// kIOMainPortDefault was introduced in macOS 12.0. Keep compatibility with
// older deployment targets supported by the Agent.
#if __MAC_OS_X_VERSION_MIN_REQUIRED >= 120000
#define IOKIT_MAIN_PORT kIOMainPortDefault
#else
#define IOKIT_MAIN_PORT kIOMasterPortDefault
#endif

static bool copyModel(CFTypeRef value, char *output, size_t outputLength) {
    if (value == NULL || outputLength == 0) {
        return false;
    }

    if (CFGetTypeID(value) == CFStringGetTypeID()) {
        return CFStringGetCString((CFStringRef)value, output, outputLength, kCFStringEncodingUTF8);
    }

    if (CFGetTypeID(value) == CFDataGetTypeID()) {
        CFDataRef data = (CFDataRef)value;
        CFIndex length = CFDataGetLength(data);
        if (length <= 0) {
            return false;
        }
        size_t copied = (size_t)length < outputLength - 1 ? (size_t)length : outputLength - 1;
        memcpy(output, CFDataGetBytePtr(data), copied);
        output[copied] = '\0';
        return true;
    }

    return false;
}

static bool copyInt64(CFTypeRef value, int64_t *output) {
    if (value == NULL || CFGetTypeID(value) != CFNumberGetTypeID()) {
        return false;
    }
    return CFNumberGetValue((CFNumberRef)value, kCFNumberSInt64Type, output);
}

static bool copyDouble(CFTypeRef value, double *output) {
    if (value == NULL || CFGetTypeID(value) != CFNumberGetTypeID()) {
        return false;
    }
    return CFNumberGetValue((CFNumberRef)value, kCFNumberDoubleType, output);
}

static void collectDevice(io_service_t service, DDAGXDeviceInfo *device, uint32_t *propertyReadErrors) {
    CFMutableDictionaryRef properties = NULL;
    kern_return_t result = IORegistryEntryCreateCFProperties(
        service,
        &properties,
        kCFAllocatorDefault,
        kNilOptions);
    if (result != KERN_SUCCESS || properties == NULL) {
        (*propertyReadErrors)++;
        return;
    }

    CFTypeRef model = CFDictionaryGetValue(properties, CFSTR("model"));
    device->hasModel = copyModel(model, device->model, sizeof(device->model));

    CFTypeRef coreCount = CFDictionaryGetValue(properties, CFSTR("gpu-core-count"));
    device->hasCoreCount = copyInt64(coreCount, &device->coreCount);

    // AGX publishes these properties through public IOKit APIs, but their names and
    // semantics are not a documented Apple contract. Keep every field optional so
    // an OS or driver change drops only the affected metric.
    CFTypeRef statistics = CFDictionaryGetValue(properties, CFSTR("PerformanceStatistics"));
    if (statistics != NULL && CFGetTypeID(statistics) == CFDictionaryGetTypeID()) {
        CFTypeRef utilization = CFDictionaryGetValue(
            (CFDictionaryRef)statistics,
            CFSTR("Device Utilization %"));
        device->hasUtilization = copyDouble(utilization, &device->utilization);

        CFTypeRef allocatedSystemMemory = CFDictionaryGetValue(
            (CFDictionaryRef)statistics,
            CFSTR("Alloc system memory"));
        device->hasAllocatedSystemMemory = copyInt64(
            allocatedSystemMemory,
            &device->allocatedSystemMemory);

        CFTypeRef inUseSystemMemory = CFDictionaryGetValue(
            (CFDictionaryRef)statistics,
            CFSTR("In use system memory"));
        device->hasInUseSystemMemory = copyInt64(
            inUseSystemMemory,
            &device->inUseSystemMemory);
    }

    CFRelease(properties);
}

DDAGXCollectionResult dd_collect_agx_devices(DDAGXDeviceInfo *devices, uint32_t capacity) {
    @autoreleasepool {
        DDAGXCollectionResult collection = {0};
        if (devices == NULL || capacity == 0) {
            collection.error = kIOReturnBadArgument;
            return collection;
        }

        io_iterator_t iterator = IO_OBJECT_NULL;
        kern_return_t result = IOServiceGetMatchingServices(
            IOKIT_MAIN_PORT,
            IOServiceMatching("AGXAccelerator"),
            &iterator);
        if (result != KERN_SUCCESS) {
            collection.error = result;
            return collection;
        }

        io_service_t service;
        while ((service = IOIteratorNext(iterator)) != IO_OBJECT_NULL) {
            if (collection.count >= capacity) {
                collection.truncated = true;
                IOObjectRelease(service);
                break;
            }

            collectDevice(service, &devices[collection.count], &collection.propertyReadErrors);
            collection.count++;
            IOObjectRelease(service);
        }

        IOObjectRelease(iterator);
        return collection;
    }
}
