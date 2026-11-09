// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#include "shutdown_cause_iokit_darwin.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <mach/mach.h>
#include <os/log.h>
#include <stdbool.h>
#include <string.h>

#define DD_PMU_MAX_SERVICES 16
#define DD_PMU_TOKEN_SEPARATOR '\x1f'

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

static bool dd_pkg_notableevents_contains_token(
    const char *buffer,
    size_t written,
    const char *token,
    size_t token_length) {
    size_t start = 0;
    while (start < written) {
        size_t end = start;
        while (end < written && buffer[end] != DD_PMU_TOKEN_SEPARATOR) {
            end++;
        }
        if (end - start == token_length && memcmp(buffer + start, token, token_length) == 0) {
            return true;
        }
        start = end + 1;
    }
    return false;
}

// dd_pkg_notableevents_append_tokens flattens one service's IOPMUBootFaultInfo
// array into buffer, continuing the separator-delimited sequence from earlier
// calls (the caller trims the final trailing separator once all services are
// processed). Returns 0 on success, -2 if buffer is too small, -3 if the
// array can't be rendered in full — the caller drops the whole service on a
// non-zero status rather than pass a partial token set to classification. A
// token already present is skipped; an oversized token is truncated rather
// than dropping its service.
static int dd_pkg_notableevents_append_tokens(
    CFArrayRef tokens,
    char *buffer,
    size_t size,
    size_t *written,
    size_t *token_count) {
    CFIndex count = CFArrayGetCount(tokens);
    if (count > DD_PMU_MAX_TOKENS_PER_SERVICE) {
        return -3;
    }

    for (CFIndex i = 0; i < count; i++) {
        CFTypeRef element = CFArrayGetValueAtIndex(tokens, i);
        // A non-string element cannot be a fault token, so stepping over it
        // loses nothing: the property is documented as an array of strings.
        if (element == NULL || CFGetTypeID(element) != CFStringGetTypeID()) {
            continue;
        }
        CFStringRef text = (CFStringRef)element;

        // Truncate to DD_PMU_MAX_TOKEN_CHARS - 1 (content only, separator
        // excluded) rather than drop the whole service.
        CFIndex length = CFStringGetLength(text);
        CFStringRef bounded = text;
        if (length > DD_PMU_MAX_TOKEN_CHARS - 1) {
            bounded = CFStringCreateWithSubstring(
                kCFAllocatorDefault,
                text,
                CFRangeMake(0, DD_PMU_MAX_TOKEN_CHARS - 1));
            if (bounded == NULL) {
                continue;
            }
            length = DD_PMU_MAX_TOKEN_CHARS - 1;
        }

        CFIndex capacity = CFStringGetMaximumSizeForEncoding(
            length,
            kCFStringEncodingUTF8);
        if (capacity <= 0) {
            if (bounded != text) {
                CFRelease(bounded);
            }
            continue;
        }
        capacity += 1;

        char token[(DD_PMU_MAX_TOKEN_CHARS - 1) * 4 + 1];
        if ((size_t)capacity > sizeof(token) ||
            !CFStringGetCString(bounded, token, sizeof(token), kCFStringEncodingUTF8)) {
            if (bounded != text) {
                CFRelease(bounded);
            }
            return -3;
        }
        if (bounded != text) {
            CFRelease(bounded);
        }
        if (token[0] == '\0') {
            continue;
        }

        size_t token_length = strlen(token);
        if (dd_pkg_notableevents_contains_token(buffer, *written, token, token_length)) {
            continue;
        }

        if (!dd_pkg_notableevents_append(buffer, size, written, token, token_length)) {
            return -2;
        }
        const char separator = DD_PMU_TOKEN_SEPARATOR;
        if (!dd_pkg_notableevents_append(buffer, size, written, &separator, 1)) {
            return -2;
        }
        *token_count += 1;
    }

    return 0;
}

int dd_pkg_notableevents_read_pmu_boot_fault_info(
    char *buffer,
    size_t size,
    size_t *written) {
    if (buffer == NULL || size == 0 || written == NULL) {
        return -1;
    }
    *written = 0;

    io_iterator_t iterator = IO_OBJECT_NULL;
    kern_return_t result = IORegistryCreateIterator(
        MACH_PORT_NULL,
        kIOServicePlane,
        kIORegistryIterateRecursively,
        &iterator);
    if (result != KERN_SUCCESS) {
        return -1;
    }

    size_t services = 0;
    size_t distinct_tokens = 0;
    io_object_t entry = IO_OBJECT_NULL;
    while ((entry = IOIteratorNext(iterator)) != IO_OBJECT_NULL) {
        if (services >= DD_PMU_MAX_SERVICES) {
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
        size_t candidate_tokens = distinct_tokens;
        int status = dd_pkg_notableevents_append_tokens(property, buffer, size, &candidate, &candidate_tokens);
        CFRelease(property);
        if (status != 0) {
            // Drop just this service's tokens and keep walking, rather than
            // discard everything already accumulated in *written.
            os_log_error(OS_LOG_DEFAULT,
                "dd_pkg_notableevents: dropping IOPMUBootFaultInfo for one service (status %d)",
                status);
            continue;
        }

        *written = candidate;
        distinct_tokens = candidate_tokens;
        services += 1;

        // once the cross-service union reaches it, every possible token has
        // already been seen, so no remaining service can add a new one.
        if (distinct_tokens >= DD_PMU_MAX_TOKENS_PER_SERVICE) {
            break;
        }
    }

    IOObjectRelease(iterator);
    if (*written > 0 && buffer[*written - 1] == DD_PMU_TOKEN_SEPARATOR) {
        // Trim the trailing separator left by the last written token.
        *written -= 1;
    }
    return 0;
}
