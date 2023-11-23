// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http2

import (
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type kernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// http2requests Count of HTTP/2 requests seen
	http2requests *libtelemetry.Counter
	// http2responses Count of HTTP/2 responses seen
	http2responses *libtelemetry.Counter
	// endOfStream Count of END_OF_STREAM flags seen
	endOfStream *libtelemetry.Counter
	// endOfStreamRST Count of RST flags seen
	endOfStreamRST *libtelemetry.Counter
	// pathSizeBucket Count of path sizes is less or equal than 120
	pathSizeBucket0 *libtelemetry.Counter
	// pathSizeBucket1 Count of path sizes between 121-130 bytes
	pathSizeBucket1 *libtelemetry.Counter
	// pathSizeBucket2 Count of path sizes between 131-140 bytes
	pathSizeBucket2 *libtelemetry.Counter
	// pathSizeBucket3 Count of path sizes between 141-150 bytes
	pathSizeBucket3 *libtelemetry.Counter
	// pathSizeBucket4 Count of path sizes between 151-160 bytes
	pathSizeBucket4 *libtelemetry.Counter
	// pathSizeBucket5 Count of path sizes between 161-179 bytes
	pathSizeBucket5 *libtelemetry.Counter
	// pathSizeBucket6 Count of path is larger or equal to 180
	pathSizeBucket6 *libtelemetry.Counter
	// pathExceedsFrame Count of times we couldn't retrieve the path due to reaching the end of the frame.
	pathExceedsFrame *libtelemetry.Counter
	// exceedingMaxInterestingFrames Count of times we reached the max number of frames per iteration.
	exceedingMaxInterestingFrames *libtelemetry.Counter
	// exceedingMaxFramesToFilter Count of times we have left with more frames to filter than the max number of frames to filter.
	exceedingMaxFramesToFilter *libtelemetry.Counter
}

// newHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func newHTTP2KernelTelemetry() *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.http2", libtelemetry.OptPrometheus)
	return &kernelTelemetry{
		metricGroup: metricGroup,

		http2requests:                 metricGroup.NewCounter("requests"),
		http2responses:                metricGroup.NewCounter("responses"),
		endOfStream:                   metricGroup.NewCounter("eos"),
		endOfStreamRST:                metricGroup.NewCounter("rst"),
		pathExceedsFrame:              metricGroup.NewCounter("path_exceeds_frame"),
		pathSizeBucket0:               metricGroup.NewCounter("path_size_bucket_0"),
		pathSizeBucket1:               metricGroup.NewCounter("path_size_bucket_1"),
		pathSizeBucket2:               metricGroup.NewCounter("path_size_bucket_2"),
		pathSizeBucket3:               metricGroup.NewCounter("path_size_bucket_3"),
		pathSizeBucket4:               metricGroup.NewCounter("path_size_bucket_4"),
		pathSizeBucket5:               metricGroup.NewCounter("path_size_bucket_5"),
		pathSizeBucket6:               metricGroup.NewCounter("path_size_bucket_6"),
		exceedingMaxInterestingFrames: metricGroup.NewCounter("exceeding_max_interesting_frames"),
		exceedingMaxFramesToFilter:    metricGroup.NewCounter("exceeding_max_frames_to_filter")}
}

func (t *kernelTelemetry) Log() {
	log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
}
