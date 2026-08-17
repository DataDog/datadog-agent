// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#include "shutdown_cause_iokit_darwin.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <mach/mach.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>

// The IORegistry is machine-controlled, so every traversal bound is explicit
// even though this runs as root inside system-probe.
#define DD_PMU_MAX_SERVICES 64
#define DD_PMU_MAX_TOKENS_PER_SERVICE 128

// DD_PMU_MAX_TOKEN_CHARS bounds one token's length, and with it the render
// buffer sized from that length. It mirrors maxShutdownTokenBytes in the Go
// classifier: a UTF-8 encoding is never shorter than the string's UTF-16
// length, so a longer string cannot pass that limit either. Rejecting it here
// declines the same payload the classifier would, before its length can decide
// an allocation size.
#define DD_PMU_MAX_TOKEN_CHARS 128

// DD_PMU_TOKEN_INLINE_BYTES is the stack fast path for the common short token,
// not a maximum: a token longer than this is rendered through the heap rather
// than shortened. The longest token on the measured hardware is 35 bytes.
#define DD_PMU_TOKEN_INLINE_BYTES 128

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
// array into buffer. Returns 0 on success, -2 when buffer was too small and -3
// when the array could not be rendered in full.
//
// A token is never shortened and never skipped for its length. Classification
// runs over the whole set and a missing token can change the elected class, so
// an incomplete set has to fail the read instead of passing as complete.
static int dd_pkg_notableevents_append_tokens(
    CFArrayRef tokens,
    char *buffer,
    size_t size,
    size_t *written) {
    CFIndex count = CFArrayGetCount(tokens);
    if (count > DD_PMU_MAX_TOKENS_PER_SERVICE) {
        return -3;
    }

    size_t emitted = 0;
    for (CFIndex i = 0; i < count; i++) {
        CFTypeRef element = CFArrayGetValueAtIndex(tokens, i);
        // A non-string element cannot be a fault token, so stepping over it
        // loses nothing: the property is documented as an array of strings.
        if (element == NULL || CFGetTypeID(element) != CFStringGetTypeID()) {
            continue;
        }
        CFStringRef text = (CFStringRef)element;

        CFIndex length = CFStringGetLength(text);
        if (length > DD_PMU_MAX_TOKEN_CHARS) {
            return -3;
        }

        // Sized from the string rather than from a fixed maximum, so the buffer
        // cannot be the reason a token fails to render. The length is bounded
        // above, which is what keeps the sizing from reaching malloc unchecked.
        CFIndex capacity = CFStringGetMaximumSizeForEncoding(
            length,
            kCFStringEncodingUTF8);
        if (capacity <= 0) {
            continue;
        }
        capacity += 1;

        char inline_token[DD_PMU_TOKEN_INLINE_BYTES];
        char *heap_token = NULL;
        char *token = inline_token;
        if ((size_t)capacity > sizeof(inline_token)) {
            heap_token = malloc((size_t)capacity);
            if (heap_token == NULL) {
                return -3;
            }
            token = heap_token;
        }

        // free(NULL) is a no-op, which is what keeps the inline path free of a
        // second exit shape.
        if (!CFStringGetCString(text, token, capacity, kCFStringEncodingUTF8)) {
            free(heap_token);
            return -3;
        }
        if (token[0] == '\0') {
            free(heap_token);
            continue;
        }

        if (emitted > 0) {
            const char separator = DD_PMU_TOKEN_SEPARATOR;
            if (!dd_pkg_notableevents_append(buffer, size, written, &separator, 1)) {
                free(heap_token);
                return -2;
            }
        }
        if (!dd_pkg_notableevents_append(buffer, size, written, token, strlen(token))) {
            free(heap_token);
            return -2;
        }
        emitted++;
        free(heap_token);
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
