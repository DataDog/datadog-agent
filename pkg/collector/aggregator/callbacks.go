// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import "unsafe"

/*
#cgo CFLAGS: -I "${SRCDIR}/../../../rtloader/include"

#include <stdbool.h>

#include "rtloader_types.h"

// The submit callbacks are exported by this package (aggregator.go). Declaring
// them here lets us take their address in C. Because the definitions live in
// the same cgo package they are resolved in the cgo intermediate link, which
// the MinGW/Windows PE linker requires (unlike ELF/Mach-O it cannot leave them
// undefined). Consumers (ffi, python) therefore must not reference these
// symbols from their own cgo preambles.
extern void SubmitMetric(char *, metric_type_t, char *, double, char **, char *, bool);
extern void SubmitServiceCheck(char *, char *, int, char **, char *, char *);
extern void SubmitEvent(char *, event_t *);
extern void SubmitHistogramBucket(char *, char *, long long, float, float, int, char *, char **, bool);
extern void SubmitEventPlatformEvent(char *, char *, int, char *);

static void *submitMetricPtr(void)             { return (void *)SubmitMetric; }
static void *submitServiceCheckPtr(void)       { return (void *)SubmitServiceCheck; }
static void *submitEventPtr(void)              { return (void *)SubmitEvent; }
static void *submitHistogramBucketPtr(void)    { return (void *)SubmitHistogramBucket; }
static void *submitEventPlatformEventPtr(void) { return (void *)SubmitEventPlatformEvent; }
*/
import "C"

// Callbacks holds the addresses of the aggregator submit callbacks as opaque C
// function pointers, so consumers can register them without referencing these
// cross-package symbols in their own cgo preambles.
type Callbacks struct {
	Metric             unsafe.Pointer
	ServiceCheck       unsafe.Pointer
	Event              unsafe.Pointer
	HistogramBucket    unsafe.Pointer
	EventPlatformEvent unsafe.Pointer
}

// GetCallbacks returns the submit callback pointers owned by this package.
func GetCallbacks() Callbacks {
	return Callbacks{
		Metric:             unsafe.Pointer(C.submitMetricPtr()),
		ServiceCheck:       unsafe.Pointer(C.submitServiceCheckPtr()),
		Event:              unsafe.Pointer(C.submitEventPtr()),
		HistogramBucket:    unsafe.Pointer(C.submitHistogramBucketPtr()),
		EventPlatformEvent: unsafe.Pointer(C.submitEventPlatformEventPtr()),
	}
}
