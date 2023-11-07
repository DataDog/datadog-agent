// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http2

import (
	"fmt"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type KernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// http2requests             Count of HTTP/2 requests seen
	// http2responses            Count of HTTP/2 responses seen
	// endOfStreamEOS            Count of END_OF_STREAM flags seen
	// endOfStreamRST            Count of RST flags seen
	// largePathInDelta          Count of path sizes between 120-180 bytes
	// largePathOutsideDelta     Count of path sizes greater than 180 bytes
	// strLenGreaterThanFrameLoc Count of times we couldn't retrieve the path due to reaching the end of the frame.
	// frameRemainder            Count of frames sent over more than one frame.

	http2requests         *libtelemetry.Gauge
	http2responses        *libtelemetry.Gauge
	endOfStreamEOS        *libtelemetry.Gauge
	endOfStreamRST        *libtelemetry.Gauge
	largePathInDelta      *libtelemetry.Gauge
	largePathOutsideDelta *libtelemetry.Gauge
	strLenExceedsFrame    *libtelemetry.Gauge
	frameRemainder        *libtelemetry.Gauge
}

// NewHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func NewHTTP2KernelTelemetry(protocol string) *KernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s", protocol))
	return &KernelTelemetry{
		metricGroup: metricGroup,

		// todo: changed it from OptStatsd to OptPrometheus
		http2requests:         metricGroup.NewGauge("http2requests", libtelemetry.OptStatsd),
		http2responses:        metricGroup.NewGauge("http2responses", libtelemetry.OptStatsd),
		endOfStreamEOS:        metricGroup.NewGauge("endOfStreamEOS", libtelemetry.OptStatsd),
		endOfStreamRST:        metricGroup.NewGauge("endOfStreamRST", libtelemetry.OptStatsd),
		strLenExceedsFrame:    metricGroup.NewGauge("strLenExceedsFrame", libtelemetry.OptStatsd),
		largePathInDelta:      metricGroup.NewGauge("largePathInDelta", libtelemetry.OptStatsd),
		largePathOutsideDelta: metricGroup.NewGauge("largePathOutsideDelta", libtelemetry.OptStatsd),
		frameRemainder:        metricGroup.NewGauge("frameRemainder", libtelemetry.OptStatsd),
	}
}

func (t *KernelTelemetry) Log() {
	log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
}
