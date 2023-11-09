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

type kernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// http2requests             Count of HTTP/2 requests seen
	// http2responses            Count of HTTP/2 responses seen
	// endOfStreamEOS            Count of END_OF_STREAM flags seen
	// endOfStreamRST            Count of RST flags seen

	// pathSizeBucket0           Count of path sizes is less or equal than 120
	// pathSizeBucket1           Count of path sizes between 121-130 bytes
	// pathSizeBucket2           Count of path sizes between 131-140 bytes
	// pathSizeBucket3           Count of path sizes between 141-150 bytes
	// pathSizeBucket4           Count of path sizes between 151-160 bytes
	// pathSizeBucket5           Count of path sizes between 161-179 bytes
	// pathSizeBucket6           Count of path is larger or equal to 180

	// strLenGreaterThanFrameLoc Count of times we couldn't retrieve the path due to reaching the end of the frame.
	// frameRemainder            Count of frames sent over more than one frame.
	// maxFramesIteration		 Count of times we reached the max number of frames per iteration.
	// iterationLimit		     Count of times we reached the max number of frames per iteration.
	// maxFramesToFilter		 Count of times we have left with more frames to filter than the max number of frames to filter.

	http2requests      *libtelemetry.Gauge
	http2responses     *libtelemetry.Gauge
	endOfStreamEOS     *libtelemetry.Gauge
	endOfStreamRST     *libtelemetry.Gauge
	pathSizeBucket0    *libtelemetry.Gauge
	pathSizeBucket1    *libtelemetry.Gauge
	pathSizeBucket2    *libtelemetry.Gauge
	pathSizeBucket3    *libtelemetry.Gauge
	pathSizeBucket4    *libtelemetry.Gauge
	pathSizeBucket5    *libtelemetry.Gauge
	pathSizeBucket6    *libtelemetry.Gauge
	strLenExceedsFrame *libtelemetry.Gauge
	frameRemainder     *libtelemetry.Gauge
	maxFramesIteration *libtelemetry.Gauge
	maxFramesToFilter  *libtelemetry.Gauge
}

// newHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func newHTTP2KernelTelemetry(protocol string) *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s", protocol))
	return &kernelTelemetry{
		metricGroup: metricGroup,

		// todo: changed it from OptStatsd to OptPrometheus
		http2requests:      metricGroup.NewGauge("http2requests", libtelemetry.OptStatsd),
		http2responses:     metricGroup.NewGauge("http2responses", libtelemetry.OptStatsd),
		endOfStreamEOS:     metricGroup.NewGauge("endOfStreamEOS", libtelemetry.OptStatsd),
		endOfStreamRST:     metricGroup.NewGauge("endOfStreamRST", libtelemetry.OptStatsd),
		strLenExceedsFrame: metricGroup.NewGauge("strLenExceedsFrame", libtelemetry.OptStatsd),
		pathSizeBucket0:    metricGroup.NewGauge("pathSizeBucket0", libtelemetry.OptStatsd),
		pathSizeBucket1:    metricGroup.NewGauge("pathSizeBucket1", libtelemetry.OptStatsd),
		pathSizeBucket2:    metricGroup.NewGauge("pathSizeBucket2", libtelemetry.OptStatsd),
		pathSizeBucket3:    metricGroup.NewGauge("pathSizeBucket3", libtelemetry.OptStatsd),
		pathSizeBucket4:    metricGroup.NewGauge("pathSizeBucket4", libtelemetry.OptStatsd),
		pathSizeBucket5:    metricGroup.NewGauge("pathSizeBucket5", libtelemetry.OptStatsd),
		pathSizeBucket6:    metricGroup.NewGauge("pathSizeBucket6", libtelemetry.OptStatsd),
		frameRemainder:     metricGroup.NewGauge("frameRemainder", libtelemetry.OptStatsd),
		maxFramesIteration: metricGroup.NewGauge("maxFramesIteration", libtelemetry.OptStatsd),
		maxFramesToFilter:  metricGroup.NewGauge("maxFramesToFilter", libtelemetry.OptStatsd)}
}

func (t *kernelTelemetry) Log() {
	log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
}
