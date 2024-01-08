// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"strconv"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type kernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// http2requests Count of HTTP/2 requests seen
	http2requests *libtelemetry.Gauge
	// http2responses Count of HTTP/2 responses seen
	http2responses *libtelemetry.Gauge
	// endOfStream Count of END_OF_STREAM flags seen
	endOfStream *libtelemetry.Gauge
	// endOfStreamRST Count of RST flags seen
	endOfStreamRST *libtelemetry.Gauge
	// pathSizeBucket Count of path sizes divided into buckets.
	pathSizeBucket [http2PathBuckets + 1]*libtelemetry.Gauge
	// pathExceedsFrame Count of times we couldn't retrieve the path due to reaching the end of the frame.
	pathExceedsFrame *libtelemetry.Gauge
	// exceedingMaxInterestingFrames Count of times we reached the max number of frames per iteration.
	exceedingMaxInterestingFrames *libtelemetry.Gauge
	// exceedingMaxFramesToFilter Count of times we have left with more frames to filter than the max number of frames to filter.
	exceedingMaxFramesToFilter *libtelemetry.Gauge
}

// newHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func newHTTP2KernelTelemetry() *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.http2", libtelemetry.OptPrometheus)
	http2KernelTel := &kernelTelemetry{
		metricGroup:                   metricGroup,
		http2requests:                 metricGroup.NewGauge("requests"),
		http2responses:                metricGroup.NewGauge("responses"),
		endOfStream:                   metricGroup.NewGauge("eos"),
		endOfStreamRST:                metricGroup.NewGauge("rst"),
		pathExceedsFrame:              metricGroup.NewGauge("path_exceeds_frame"),
		exceedingMaxInterestingFrames: metricGroup.NewGauge("exceeding_max_interesting_frames"),
		exceedingMaxFramesToFilter:    metricGroup.NewGauge("exceeding_max_frames_to_filter")}
	for bucketIndex := range http2KernelTel.pathSizeBucket {
		http2KernelTel.pathSizeBucket[bucketIndex] = metricGroup.NewGauge("path_size_bucket_" + (strconv.Itoa(bucketIndex + 1)))
	}

	return http2KernelTel
}

// update updates the kernel metrics with the given telemetry.
func (t *kernelTelemetry) update(tel *HTTP2Telemetry) {
	t.http2requests.Set(int64(tel.Request_seen))
	t.http2responses.Set(int64(tel.Response_seen))
	t.endOfStream.Set(int64(tel.End_of_stream))
	t.endOfStreamRST.Set(int64(tel.End_of_stream_rst))
	t.pathExceedsFrame.Set(int64(tel.Path_exceeds_frame))
	t.exceedingMaxInterestingFrames.Set(int64(tel.Exceeding_max_interesting_frames))
	t.exceedingMaxFramesToFilter.Set(int64(tel.Exceeding_max_frames_to_filter))
	for bucketIndex := range t.pathSizeBucket {
		t.pathSizeBucket[bucketIndex].Set(int64(tel.Path_size_bucket[bucketIndex]))
	}
}

func (t *kernelTelemetry) Log() {
	log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
}
