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

// The IORegistry is machine-controlled, so every traversal bound is explicit
// even though this runs as root inside system-probe.
#define DD_PMU_MAX_SERVICES 16

// DD_PMU_MAX_TOKENS_PER_SERVICE and DD_PMU_MAX_TOKEN_CHARS are sized from the
// largest payload observed on real hardware (the 80-token dictionary in
// allPMUFaultTokens(), longest token 35 bytes), with a margin rather than an
// exact fit: 10% over the token count, 50% over the longest token's length.
// A service that exceeds DD_PMU_MAX_TOKENS_PER_SERVICE is dropped rather than
// failing the whole read; see the caller in
// dd_pkg_notableevents_read_pmu_boot_fault_info. DD_PMU_MAX_TOKEN_CHARS counts
// a token's content plus the one separator byte that follows it, so a token
// longer than that is truncated to fit rather than dropping its service.
#define DD_PMU_MAX_TOKENS_PER_SERVICE 88
#define DD_PMU_MAX_TOKEN_CHARS 53

#define DD_PMU_TOKEN_SEPARATOR '\x1f'

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

// dd_pkg_notableevents_contains_token reports whether token already occurs as
// a complete, separator-delimited entry in buffer[0, written), the same way
// strings.Split on the Go side would see it: this is how a token already
// written by an earlier element, or by an earlier service, is recognized as a
// repeat rather than re-scanned some other way.
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
// array into buffer, continuing the flat, DD_PMU_TOKEN_SEPARATOR-delimited
// sequence started by an earlier call: every token is followed by a
// separator, including the last one written by the last service, so the
// caller trims that single trailing separator once every service has been
// processed. Returns 0 on success, -2 when buffer was too small and -3 when
// the array could not be rendered in full.
//
// A token longer than DD_PMU_MAX_TOKEN_CHARS - 1 is truncated to fit rather
// than dropped. A service whose array trips DD_PMU_MAX_TOKENS_PER_SERVICE or
// the buffer's remaining capacity is dropped by the caller in its entirety
// and logged, rather than passing a partial token set into classification. A
// token already present in the buffer, whether from an earlier element in
// this same array or from a service processed earlier, is skipped rather
// than written a second time.
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

        // DD_PMU_MAX_TOKEN_CHARS counts the token's content plus the one
        // separator byte written after it, so content itself is bounded to
        // one char less. A longer token is truncated to that bound rather
        // than dropping the whole service over it.
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

        // Sized for the worst-case UTF-8 expansion of DD_PMU_MAX_TOKEN_CHARS - 1
        // UTF-16 code units, so no allocation is ever needed to render a token
        // that has already been bounded above.
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

// dd_pkg_notableevents_read_pmu_boot_fault_info walks the IOService plane and
// flattens every IOPMUBootFaultInfo array it finds. The walk can end before
// the plane or DD_PMU_MAX_SERVICES is exhausted: once DD_PMU_MAX_TOKENS_PER_SERVICE
// distinct tokens have been collected, no remaining service can add one that
// is not already present.
int dd_pkg_notableevents_read_pmu_boot_fault_info(
    char *buffer,
    size_t size,
    size_t *written) {
    if (buffer == NULL || size == 0 || written == NULL) {
        return -1;
    }
    *written = 0;

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
            // A bound violation is scoped to this one service: drop its
            // tokens and keep walking the plane rather than discarding
            // every service already accumulated in *written.
            os_log_error(OS_LOG_DEFAULT,
                "dd_pkg_notableevents: dropping IOPMUBootFaultInfo for one service (status %d)",
                status);
            continue;
        }

        *written = candidate;
        distinct_tokens = candidate_tokens;
        services += 1;

        // DD_PMU_MAX_TOKENS_PER_SERVICE is sized with a margin over the full
        // known real-world PMU fault dictionary, so once that many distinct
        // tokens have been collected, no remaining service can contribute one
        // that is not already in the buffer.
        if (distinct_tokens >= DD_PMU_MAX_TOKENS_PER_SERVICE) {
            break;
        }
    }

    IOObjectRelease(iterator);
    if (*written > 0 && buffer[*written - 1] == DD_PMU_TOKEN_SEPARATOR) {
        // Every emitted token is followed by a separator, so the very last
        // token wrote one that has nothing after it. Trim it here rather
        // than avoid writing it in dd_pkg_notableevents_append_tokens, which
        // would need to know whether more tokens follow from a later service.
        *written -= 1;
    }
    return 0;
}
