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

// tlsAwareCounter is a TLS aware counter, it has a plain counter and a counter for TLS.
// It enables the use of a single metric that increments based on the encryption, avoiding the need for separate metrics for eash use-case.
type tlsAwareCounter struct {
	counterPlain *libtelemetry.Counter
	counterTLS   *libtelemetry.Counter
}

// newTLSAwareCounter creates and returns a new instance of TLSCounter
func newTLSAwareCounter(metricGroup *libtelemetry.MetricGroup, metricName string, tags ...string) *tlsAwareCounter {
	return &tlsAwareCounter{
		counterPlain: metricGroup.NewCounter(metricName, append(tags, "encrypted:false")...),
		counterTLS:   metricGroup.NewCounter(metricName, append(tags, "encrypted:true")...),
	}
}

// add adds the given delta to the counter based on the encryption.
func (c *tlsAwareCounter) add(delta int64, isTLS bool) {
	if isTLS {
		c.counterTLS.Add(delta)
		return
	}
	c.counterPlain.Add(delta)
}

// get returns the counter value based on the encryption.
func (c *tlsAwareCounter) get(isTLS bool) int64 {
	if isTLS {
		return c.counterTLS.Get()
	}
	return c.counterPlain.Get()
}

type kernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// http2requests Count of HTTP/2 requests seen
	http2requests *tlsAwareCounter
	// http2responses Count of HTTP/2 responses seen
	http2responses *tlsAwareCounter
	// endOfStream Count of END_OF_STREAM flags seen
	endOfStream *tlsAwareCounter
	// endOfStreamRST Count of RST flags seen
	endOfStreamRST *tlsAwareCounter
	// pathSizeBucket Count of path sizes divided into buckets.
	pathSizeBucket [http2PathBuckets + 1]*tlsAwareCounter
	// literalValueExceedsFrame Count of times we couldn't retrieve the literal value due to reaching the end of the frame.
	literalValueExceedsFrame *tlsAwareCounter
	// exceedingMaxInterestingFrames Count of times we reached the max number of frames per iteration.
	exceedingMaxInterestingFrames *tlsAwareCounter
	// exceedingMaxFramesToFilter Count of times we have left with more frames to filter than the max number of frames to filter.
	exceedingMaxFramesToFilter *tlsAwareCounter
	// exceedingDataEnd Count of times we tried to read data beyond the end of the buffer.
	exceedingDataEnd *tlsAwareCounter

	// telemetryLastState represents the latest HTTP2 eBPF Kernel telemetry observed from the kernel
	telemetryLastState HTTP2Telemetry
}

// newHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func newHTTP2KernelTelemetry() *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.http2", libtelemetry.OptPrometheus)
	http2KernelTel := &kernelTelemetry{
		metricGroup:                   metricGroup,
		http2requests:                 newTLSAwareCounter(metricGroup, "requests"),
		http2responses:                newTLSAwareCounter(metricGroup, "responses"),
		endOfStream:                   newTLSAwareCounter(metricGroup, "eos"),
		endOfStreamRST:                newTLSAwareCounter(metricGroup, "rst"),
		literalValueExceedsFrame:      newTLSAwareCounter(metricGroup, "literal_value_exceeds_frame"),
		exceedingMaxInterestingFrames: newTLSAwareCounter(metricGroup, "exceeding_max_interesting_frames"),
		exceedingMaxFramesToFilter:    newTLSAwareCounter(metricGroup, "exceeding_max_frames_to_filter"),
		exceedingDataEnd:              newTLSAwareCounter(metricGroup, "exceeding_data_end")}

	for bucketIndex := range http2KernelTel.pathSizeBucket {
		http2KernelTel.pathSizeBucket[bucketIndex] = newTLSAwareCounter(metricGroup, "path_size_bucket_"+(strconv.Itoa(bucketIndex+1)))
	}

	return http2KernelTel
}

// update updates the kernel metrics with the given telemetry.
func (t *kernelTelemetry) update(tel *HTTP2Telemetry, isTLS bool) {
	// We should only add the delta between the current eBPF map state and the last seen eBPF map state
	telemetryDelta := tel.Sub(t.telemetryLastState)
	t.http2requests.add(int64(telemetryDelta.Request_seen), isTLS)
	t.http2responses.add(int64(telemetryDelta.Response_seen), isTLS)
	t.endOfStream.add(int64(telemetryDelta.End_of_stream), isTLS)
	t.endOfStreamRST.add(int64(telemetryDelta.End_of_stream_rst), isTLS)
	t.literalValueExceedsFrame.add(int64(telemetryDelta.Literal_value_exceeds_frame), isTLS)
	t.exceedingMaxInterestingFrames.add(int64(telemetryDelta.Exceeding_max_interesting_frames), isTLS)
	t.exceedingMaxFramesToFilter.add(int64(telemetryDelta.Exceeding_max_frames_to_filter), isTLS)
	t.exceedingDataEnd.add(int64(telemetryDelta.Exceeding_data_end), isTLS)
	for bucketIndex := range t.pathSizeBucket {
		t.pathSizeBucket[bucketIndex].add(int64(telemetryDelta.Path_size_bucket[bucketIndex]), isTLS)
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
		Literal_value_exceeds_frame:      t.Literal_value_exceeds_frame - other.Literal_value_exceeds_frame,
		Exceeding_max_interesting_frames: t.Exceeding_max_interesting_frames - other.Exceeding_max_interesting_frames,
		Exceeding_max_frames_to_filter:   t.Exceeding_max_frames_to_filter - other.Exceeding_max_frames_to_filter,
		Exceeding_data_end:               t.Exceeding_data_end - other.Exceeding_data_end,
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
