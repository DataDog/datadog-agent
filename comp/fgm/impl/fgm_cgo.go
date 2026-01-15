// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo

package fgmimpl

/*
#cgo LDFLAGS: -lfgm_observer -ldl -lpthread -lm
#include <stdlib.h>
#include "fgm_observer.h"

// Forward declaration of the Go callback
extern void goMetricCallback(char* name, double value, char* tags_json, long long timestamp_ms, void* ctx);

// C wrapper that bridges to the Go callback
static void metric_callback_wrapper(const char* name, double value, const char* tags_json, long long timestamp_ms, void* ctx) {
    goMetricCallback((char*)name, value, (char*)tags_json, timestamp_ms, ctx);
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

type samplingContext struct {
	handle observer.Handle
	tags   []string
}

//export goMetricCallback
func goMetricCallback(
	name *C.char,
	value C.double,
	tagsJSON *C.char,
	timestampMS C.longlong,
	ctx unsafe.Pointer,
) {
	sctx := (*samplingContext)(ctx)

	metricName := C.GoString(name)
	metricValue := float64(value)
	metricTimestamp := float64(timestampMS) / 1000.0

	// Parse tags from JSON
	var rustTags []string
	if tagsJSON != nil {
		tagStr := C.GoString(tagsJSON)
		if tagStr != "" && tagStr != "[]" {
			_ = json.Unmarshal([]byte(tagStr), &rustTags)
		}
	}

	// Merge container tags with metric tags
	allTags := append(sctx.tags, rustTags...)

	// Emit to Observer
	sctx.handle.ObserveMetric(&fgmMetricView{
		name:      metricName,
		value:     metricValue,
		tags:      allTags,
		timestamp: metricTimestamp,
	})
}

func fgmInit() error {
	ret := C.fgm_init()
	if ret != 0 {
		return fmt.Errorf("fgm_init returned %d", ret)
	}
	return nil
}

func fgmShutdown() {
	C.fgm_shutdown()
}

func (c *fgmComponent) sampleContainer(tc *trackedContainer) error {
	cgroupPath := C.CString(tc.cgroupPath)
	defer C.free(unsafe.Pointer(cgroupPath))

	// Build base tags from container metadata
	baseTags := []string{
		fmt.Sprintf("container_id:%s", tc.id[:min(12, len(tc.id))]),
	}
	for k, v := range tc.labels {
		baseTags = append(baseTags, fmt.Sprintf("%s:%s", k, v))
	}

	sctx := &samplingContext{
		handle: c.observerHandle,
		tags:   baseTags,
	}

	ret := C.fgm_sample_container(
		cgroupPath,
		C.int(tc.pid),
		C.fgm_metric_callback(C.metric_callback_wrapper),
		unsafe.Pointer(sctx),
	)

	if ret != 0 {
		return fmt.Errorf("fgm_sample_container returned %d", ret)
	}

	return nil
}
