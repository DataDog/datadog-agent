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
	http2requests *libtelemetry.Counter
	// http2responses Count of HTTP/2 responses seen
	http2responses *libtelemetry.Counter
	// endOfStream Count of END_OF_STREAM flags seen
	endOfStream *libtelemetry.Counter
	// endOfStreamRST Count of RST flags seen
	endOfStreamRST *libtelemetry.Counter
	// pathSizeBucket Count of path sizes divided into buckets.
	pathSizeBucket [http2PathBuckets + 1]*libtelemetry.Counter
	// pathExceedsFrame Count of times we couldn't retrieve the path due to reaching the end of the frame.
	pathExceedsFrame *libtelemetry.Counter
	// exceedingMaxInterestingFrames Count of times we reached the max number of frames per iteration.
	exceedingMaxInterestingFrames *libtelemetry.Counter
	// exceedingMaxFramesToFilter Count of times we have left with more frames to filter than the max number of frames to filter.
	exceedingMaxFramesToFilter *libtelemetry.Counter

	// telemetryLastState represents the latest HTTP2 eBPF Kernel telemetry observed from the kernel
	telemetryLastState HTTP2Telemetry
}

// newHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func newHTTP2KernelTelemetry() *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.http2", libtelemetry.OptPrometheus)
	http2KernelTel := &kernelTelemetry{
		metricGroup:                   metricGroup,
		http2requests:                 metricGroup.NewCounter("requests"),
		http2responses:                metricGroup.NewCounter("responses"),
		endOfStream:                   metricGroup.NewCounter("eos"),
		endOfStreamRST:                metricGroup.NewCounter("rst"),
		pathExceedsFrame:              metricGroup.NewCounter("path_exceeds_frame"),
		exceedingMaxInterestingFrames: metricGroup.NewCounter("exceeding_max_interesting_frames"),
		exceedingMaxFramesToFilter:    metricGroup.NewCounter("exceeding_max_frames_to_filter")}
	for bucketIndex := range http2KernelTel.pathSizeBucket {
		http2KernelTel.pathSizeBucket[bucketIndex] = metricGroup.NewCounter("path_size_bucket_" + (strconv.Itoa(bucketIndex + 1)))
	}

	return http2KernelTel
}

// update updates the kernel metrics with the given telemetry.
func (t *kernelTelemetry) update(tel *HTTP2Telemetry) {
	// We should only add the delta between the current eBPF map state and the last seen eBPF map state
	telemetryDelta := tel.Sub(t.telemetryLastState)
	t.http2requests.Add(int64(telemetryDelta.Request_seen))
	t.http2responses.Add(int64(telemetryDelta.Response_seen))
	t.endOfStream.Add(int64(telemetryDelta.End_of_stream))
	t.endOfStreamRST.Add(int64(telemetryDelta.End_of_stream_rst))
	t.pathExceedsFrame.Add(int64(telemetryDelta.Path_exceeds_frame))
	t.exceedingMaxInterestingFrames.Add(int64(telemetryDelta.Exceeding_max_interesting_frames))
	t.exceedingMaxFramesToFilter.Add(int64(telemetryDelta.Exceeding_max_frames_to_filter))
	for bucketIndex := range t.pathSizeBucket {
		t.pathSizeBucket[bucketIndex].Add(int64(telemetryDelta.Path_size_bucket[bucketIndex]))
	}
	// Create a deep copy of the 'tel' parameter to prevent changes from the outer scope affecting the last state
	t.telemetryLastState = *tel
}

func (t *kernelTelemetry) Log() {
	log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
}

// Sub generates a new HTTP2Telemetry object by subtracting the values of this HTTP2Telemetry object from the other
func (t *HTTP2Telemetry) Sub(other HTTP2Telemetry) *HTTP2Telemetry {
	return &HTTP2Telemetry{
		Request_seen:                     t.Request_seen - other.Request_seen,
		Response_seen:                    t.Response_seen - other.Response_seen,
		End_of_stream:                    t.End_of_stream - other.End_of_stream,
		End_of_stream_rst:                t.End_of_stream_rst - other.End_of_stream_rst,
		Path_exceeds_frame:               t.Path_exceeds_frame - other.Path_exceeds_frame,
		Exceeding_max_interesting_frames: t.Exceeding_max_interesting_frames - other.Exceeding_max_interesting_frames,
		Exceeding_max_frames_to_filter:   t.Exceeding_max_frames_to_filter - other.Exceeding_max_frames_to_filter,
		Path_size_bucket:                 computePathSizeBucketDifferences(t.Path_size_bucket, other.Path_size_bucket),
	}
}

func computePathSizeBucketDifferences(pathSizeBucket, otherPathSizeBucket [8]uint64) [8]uint64 {
	var result [8]uint64

	for i := 0; i < 8; i++ {
		result[i] = pathSizeBucket[i] - otherPathSizeBucket[i]
	}

	return result
}
