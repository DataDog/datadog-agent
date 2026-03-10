// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package ffi

/*
#cgo CFLAGS: -I "${SRCDIR}/../../../../rtloader/include"
#include "ffi.h"

// Forward declarations of Go-exported bridge functions.
// These are defined in library_loader.go via //export directives.
extern void BridgeSubmitMetric(void *ctx, int metric_type, const char *name, double value, const char **tags, const char *hostname, int flush_first);
extern void BridgeSubmitServiceCheck(void *ctx, const char *name, int status, const char **tags, const char *hostname, const char *message);
extern void BridgeSubmitEvent(void *ctx, const slim_event_t *event);
extern void BridgeSubmitHistogram(void *ctx, const char *name, long long value, float lower, float upper, int monotonic, const char *hostname, const char **tags, int flush_first);
extern void BridgeSubmitEventPlatformEvent(void *ctx, const char *event, int event_len, const char *event_type);
extern void BridgeSubmitLog(void *ctx, int level, const char *message);

static callback_t build_bridge_callback(void) {
	callback_t cb;
	cb.submit_metric = BridgeSubmitMetric;
	cb.submit_service_check = BridgeSubmitServiceCheck;
	cb.submit_event = BridgeSubmitEvent;
	cb.submit_histogram = BridgeSubmitHistogram;
	cb.submit_event_platform_event = BridgeSubmitEventPlatformEvent;
	cb.submit_log = BridgeSubmitLog;
	return cb;
}
*/
import "C"

// buildCallback constructs a C callback_t struct populated with our Go bridge functions.
func buildCallback() C.callback_t {
	return C.build_bridge_callback()
}
