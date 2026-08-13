// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#include "shutdown_cause_iokit_darwin.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <mach/mach.h>
#include <stdbool.h>
#include <string.h>

// The IORegistry is machine-controlled, so every traversal bound is explicit
// even though this runs as root inside system-probe.
#define DD_PMU_MAX_SERVICES 64
#define DD_PMU_MAX_TOKENS_PER_SERVICE 128
#define DD_PMU_MAX_TOKEN_BYTES 128

#define DD_PMU_TOKEN_SEPARATOR '\x1f'
#define DD_PMU_SERVICE_SEPARATOR '\x1e'

// dd_pkg_notableevents_append copies value into buffer, reporting whether the
// remaining capacity was sufficient.
static bool dd_pkg_notableevents_append(
    char *buffer,
    size_t size,
    size_t *written,
    const char *value,
    size_t value_length) {
    if (value_length > size - *written) {
        return false;
    }
    memcpy(buffer + *written, value, value_length);
    *written += value_length;
    return true;
}

// dd_pkg_notableevents_append_tokens flattens one service's IOPMUBootFaultInfo
// array into buffer. Returns 0 on success, -2 when buffer was too small.
static int dd_pkg_notableevents_append_tokens(
    CFArrayRef tokens,
    char *buffer,
    size_t size,
    size_t *written) {
    CFIndex count = CFArrayGetCount(tokens);
    if (count > DD_PMU_MAX_TOKENS_PER_SERVICE) {
        count = DD_PMU_MAX_TOKENS_PER_SERVICE;
    }

    size_t emitted = 0;
    for (CFIndex i = 0; i < count; i++) {
        CFTypeRef element = CFArrayGetValueAtIndex(tokens, i);
        if (element == NULL || CFGetTypeID(element) != CFStringGetTypeID()) {
            continue;
        }

        char token[DD_PMU_MAX_TOKEN_BYTES];
        if (!CFStringGetCString(element, token, sizeof(token), kCFStringEncodingUTF8)) {
            continue;
        }
        if (token[0] == '\0') {
            continue;
        }

        if (emitted > 0) {
            const char separator = DD_PMU_TOKEN_SEPARATOR;
            if (!dd_pkg_notableevents_append(buffer, size, written, &separator, 1)) {
                return -2;
            }
        }
        if (!dd_pkg_notableevents_append(buffer, size, written, token, strlen(token))) {
            return -2;
        }
        emitted++;
    }

    return 0;
}

// dd_pkg_notableevents_read_pmu_boot_fault_info walks the IOService plane and
// flattens every IOPMUBootFaultInfo array it finds.
int dd_pkg_notableevents_read_pmu_boot_fault_info(
    char *buffer,
    size_t size,
    size_t *written,
    size_t *services) {
    if (buffer == NULL || size == 0 || written == NULL || services == NULL) {
        return -1;
    }
    *written = 0;
    *services = 0;

    // The property is published by a PMIC-specific class name that differs
    // across Macs, and IOKit matching cannot match on key presence alone
    // (kIOPropertyMatchKey matches key and value, and the value is per
    // machine), so the whole plane is traversed instead.
    io_iterator_t iterator = IO_OBJECT_NULL;
    // MACH_PORT_NULL selects the default port without naming the constant
    // renamed from kIOMasterPortDefault to kIOMainPortDefault in macOS 12.
    kern_return_t result = IORegistryCreateIterator(
        MACH_PORT_NULL,
        kIOServicePlane,
        kIORegistryIterateRecursively,
        &iterator);
    if (result != KERN_SUCCESS) {
        return -1;
    }

    int status = 0;
    io_object_t entry = IO_OBJECT_NULL;
    while ((entry = IOIteratorNext(iterator)) != IO_OBJECT_NULL) {
        if (*services >= DD_PMU_MAX_SERVICES) {
            IOObjectRelease(entry);
            break;
        }

        CFTypeRef property = IORegistryEntryCreateCFProperty(
            entry,
            CFSTR("IOPMUBootFaultInfo"),
            kCFAllocatorDefault,
            0);
        IOObjectRelease(entry);
        if (property == NULL) {
            continue;
        }
        if (CFGetTypeID(property) != CFArrayGetTypeID()) {
            CFRelease(property);
            continue;
        }

        size_t candidate = *written;
        if (*services > 0) {
            const char separator = DD_PMU_SERVICE_SEPARATOR;
            if (!dd_pkg_notableevents_append(buffer, size, &candidate, &separator, 1)) {
                CFRelease(property);
                status = -2;
                break;
            }
        }
        status = dd_pkg_notableevents_append_tokens(property, buffer, size, &candidate);
        CFRelease(property);
        if (status != 0) {
            break;
        }

        *written = candidate;
        *services += 1;
    }

    IOObjectRelease(iterator);
    if (status != 0) {
        *written = 0;
        *services = 0;
    }
    return status;
}
