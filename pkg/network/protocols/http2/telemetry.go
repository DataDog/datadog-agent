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
	// maxInterestingFrames		 Count of times we reached the max number of frames per iteration.
	// maxFramesToFilter		 Count of times we have left with more frames to filter than the max number of frames to filter.

	http2requests        *libtelemetry.Counter
	http2responses       *libtelemetry.Counter
	endOfStreamEOS       *libtelemetry.Counter
	endOfStreamRST       *libtelemetry.Counter
	pathSizeBucket0      *libtelemetry.Counter
	pathSizeBucket1      *libtelemetry.Counter
	pathSizeBucket2      *libtelemetry.Counter
	pathSizeBucket3      *libtelemetry.Counter
	pathSizeBucket4      *libtelemetry.Counter
	pathSizeBucket5      *libtelemetry.Counter
	pathSizeBucket6      *libtelemetry.Counter
	strLenExceedsFrame   *libtelemetry.Counter
	maxInterestingFrames *libtelemetry.Counter
	maxFramesToFilter    *libtelemetry.Counter
}

// newHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func newHTTP2KernelTelemetry(protocol string) *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s", protocol), libtelemetry.OptPrometheus)
	return &kernelTelemetry{
		metricGroup: metricGroup,

		http2requests:        metricGroup.NewCounter("requests"),
		http2responses:       metricGroup.NewCounter("responses"),
		endOfStreamEOS:       metricGroup.NewCounter("eos"),
		endOfStreamRST:       metricGroup.NewCounter("rst"),
		strLenExceedsFrame:   metricGroup.NewCounter("str_len_exceeds_frame"),
		pathSizeBucket0:      metricGroup.NewCounter("path_size_bucket_0"),
		pathSizeBucket1:      metricGroup.NewCounter("path_size_bucket_1"),
		pathSizeBucket2:      metricGroup.NewCounter("path_size_bucket_2"),
		pathSizeBucket3:      metricGroup.NewCounter("path_size_bucket_3"),
		pathSizeBucket4:      metricGroup.NewCounter("path_size_bucket_4"),
		pathSizeBucket5:      metricGroup.NewCounter("path_size_bucket_5"),
		pathSizeBucket6:      metricGroup.NewCounter("path_size_bucket_6"),
		maxInterestingFrames: metricGroup.NewCounter("max_interesting_frames"),
		maxFramesToFilter:    metricGroup.NewCounter("max_frames_to_filter")}
}

func (t *kernelTelemetry) Log() {
	log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
}
