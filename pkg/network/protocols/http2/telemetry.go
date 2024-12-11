// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"strconv"

	"github.com/cihub/seelog"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type kernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// http2requests Count of HTTP/2 requests seen
	http2requests *libtelemetry.TLSAwareCounter
	// http2responses Count of HTTP/2 responses seen
	http2responses *libtelemetry.TLSAwareCounter
	// endOfStream Count of END_OF_STREAM flags seen
	endOfStream *libtelemetry.TLSAwareCounter
	// endOfStreamRST Count of RST flags seen
	endOfStreamRST *libtelemetry.TLSAwareCounter
	// pathSizeBucket Count of path sizes divided into buckets.
	pathSizeBucket [http2PathBuckets + 1]*libtelemetry.TLSAwareCounter
	// literalValueExceedsFrame Count of times we couldn't retrieve the literal value due to reaching the end of the frame.
	literalValueExceedsFrame *libtelemetry.TLSAwareCounter
	// exceedingMaxInterestingFrames Count of times we reached the max number of frames per iteration.
	exceedingMaxInterestingFrames *libtelemetry.TLSAwareCounter
	// exceedingMaxFramesToFilter Count of times we have left with more frames to filter than the max number of frames to filter.
	exceedingMaxFramesToFilter *libtelemetry.TLSAwareCounter
	// fragmentedFrameCountRST Count of times we have seen a fragmented RST frame.
	fragmentedFrameCountRST *libtelemetry.TLSAwareCounter
	// fragmentedHeadersFrameEOSCount Count of times we have seen a fragmented headers frame with EOS.
	fragmentedHeadersFrameEOSCount *libtelemetry.TLSAwareCounter
	// fragmentedHeadersFrameCount Count of times we have seen a fragmented headers frame.
	fragmentedHeadersFrameCount *libtelemetry.TLSAwareCounter
	// fragmentedDataFrameEOSCount Count of times we have seen a fragmented data frame with EOS.
	fragmentedDataFrameEOSCount *libtelemetry.TLSAwareCounter
	// telemetryLastState represents the latest HTTP2 eBPF Kernel telemetry observed from the kernel
	telemetryLastState HTTP2Telemetry
}

// newHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func newHTTP2KernelTelemetry() *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.http2", libtelemetry.OptPrometheus)
	http2KernelTel := &kernelTelemetry{
		metricGroup:                    metricGroup,
		http2requests:                  libtelemetry.NewTLSAwareCounter(metricGroup, "requests"),
		http2responses:                 libtelemetry.NewTLSAwareCounter(metricGroup, "responses"),
		endOfStream:                    libtelemetry.NewTLSAwareCounter(metricGroup, "eos"),
		endOfStreamRST:                 libtelemetry.NewTLSAwareCounter(metricGroup, "rst"),
		literalValueExceedsFrame:       libtelemetry.NewTLSAwareCounter(metricGroup, "literal_value_exceeds_frame"),
		exceedingMaxInterestingFrames:  libtelemetry.NewTLSAwareCounter(metricGroup, "exceeding_max_interesting_frames"),
		exceedingMaxFramesToFilter:     libtelemetry.NewTLSAwareCounter(metricGroup, "exceeding_max_frames_to_filter"),
		fragmentedDataFrameEOSCount:    libtelemetry.NewTLSAwareCounter(metricGroup, "exceeding_data_end_data_eos"),
		fragmentedHeadersFrameCount:    libtelemetry.NewTLSAwareCounter(metricGroup, "exceeding_data_end_headers"),
		fragmentedHeadersFrameEOSCount: libtelemetry.NewTLSAwareCounter(metricGroup, "exceeding_data_end_headers_eos"),
		fragmentedFrameCountRST:        libtelemetry.NewTLSAwareCounter(metricGroup, "exceeding_data_end_rst")}

	for bucketIndex := range http2KernelTel.pathSizeBucket {
		http2KernelTel.pathSizeBucket[bucketIndex] = libtelemetry.NewTLSAwareCounter(metricGroup, "path_size_bucket_"+(strconv.Itoa(bucketIndex+1)))
	}

	return http2KernelTel
}

// update updates the kernel metrics with the given telemetry.
func (t *kernelTelemetry) update(tel *HTTP2Telemetry, isTLS bool) {
	// We should only add the delta between the current eBPF map state and the last seen eBPF map state
	telemetryDelta := tel.Sub(t.telemetryLastState)
	t.http2requests.Add(int64(telemetryDelta.Request_seen), isTLS)
	t.http2responses.Add(int64(telemetryDelta.Response_seen), isTLS)
	t.endOfStream.Add(int64(telemetryDelta.End_of_stream), isTLS)
	t.endOfStreamRST.Add(int64(telemetryDelta.End_of_stream_rst), isTLS)
	t.literalValueExceedsFrame.Add(int64(telemetryDelta.Literal_value_exceeds_frame), isTLS)
	t.exceedingMaxInterestingFrames.Add(int64(telemetryDelta.Exceeding_max_interesting_frames), isTLS)
	t.exceedingMaxFramesToFilter.Add(int64(telemetryDelta.Exceeding_max_frames_to_filter), isTLS)
	for bucketIndex := range t.pathSizeBucket {
		t.pathSizeBucket[bucketIndex].Add(int64(telemetryDelta.Path_size_bucket[bucketIndex]), isTLS)
	}
	// Create a deep copy of the 'tel' parameter to prevent changes from the outer scope affecting the last state
	t.telemetryLastState = *tel
}

func (t *kernelTelemetry) Log() {
	if log.ShouldLog(seelog.DebugLvl) {
		log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
	}
}

// Sub generates a new HTTP2Telemetry object by subtracting the values of this HTTP2Telemetry object from the other
func (t *HTTP2Telemetry) Sub(other HTTP2Telemetry) *HTTP2Telemetry {
	return &HTTP2Telemetry{
		Request_seen:                     t.Request_seen - other.Request_seen,
		Response_seen:                    t.Response_seen - other.Response_seen,
		End_of_stream:                    t.End_of_stream - other.End_of_stream,
		End_of_stream_rst:                t.End_of_stream_rst - other.End_of_stream_rst,
		Literal_value_exceeds_frame:      t.Literal_value_exceeds_frame - other.Literal_value_exceeds_frame,
		Exceeding_max_interesting_frames: t.Exceeding_max_interesting_frames - other.Exceeding_max_interesting_frames,
		Exceeding_max_frames_to_filter:   t.Exceeding_max_frames_to_filter - other.Exceeding_max_frames_to_filter,
		Path_size_bucket:                 computePathSizeBucketDifferences(t.Path_size_bucket, other.Path_size_bucket),
	}
}

func computePathSizeBucketDifferences(pathSizeBucket, otherPathSizeBucket [http2PathBuckets + 1]uint64) [http2PathBuckets + 1]uint64 {
	var result [http2PathBuckets + 1]uint64

	for i := 0; i < http2PathBuckets+1; i++ {
		result[i] = pathSizeBucket[i] - otherPathSizeBucket[i]
	}

	return result
}
