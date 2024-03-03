// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"strconv"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type kernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// topicNameExceedsMaxSize Count of topic names observed exceeding the maximum allowed size
	topicNameExceedsMaxSize *libtelemetry.Counter

	// pathSizeBucket Count of topic names sizes divided into buckets.
	pathSizeBucket [topicNameBuckets + 1]*libtelemetry.Counter

	// telemetryLastState represents the latest HTTP2 eBPF Kernel telemetry observed from the kernel
	telemetryLastState rawKernelTelemetry
}

// newKernelTelemetry hold Kafka kernel metrics.
func newKernelTelemetry() *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.kafka", libtelemetry.OptPrometheus)
	kafkaKernelTel := &kernelTelemetry{
		metricGroup:             metricGroup,
		topicNameExceedsMaxSize: metricGroup.NewCounter("exceeding_max_topic_size"),
	}

	for bucketIndex := range kafkaKernelTel.pathSizeBucket {
		kafkaKernelTel.pathSizeBucket[bucketIndex] = metricGroup.NewCounter("path_size_bucket_" + (strconv.Itoa(bucketIndex + 1)))
	}

	return kafkaKernelTel
}

// update updates the kernel metrics with the given telemetry.
func (t *kernelTelemetry) update(tel *rawKernelTelemetry) {
	// We should only add the delta between the current eBPF map state and the last seen eBPF map state
	telemetryDelta := tel.Sub(t.telemetryLastState)
	t.topicNameExceedsMaxSize.Add(int64(telemetryDelta.Name_exceeds_max_size))
	for bucketIndex := range t.pathSizeBucket {
		t.pathSizeBucket[bucketIndex].Add(int64(telemetryDelta.Name_size_buckets[bucketIndex]))
	}
	// Create a deep copy of the 'tel' parameter to prevent changes from the outer scope affecting the last state
	t.telemetryLastState = *tel
}

func (t *kernelTelemetry) Log() {
	log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
}

// Sub generates a new HTTP2Telemetry object by subtracting the values of this HTTP2Telemetry object from the other
func (t *rawKernelTelemetry) Sub(other rawKernelTelemetry) *rawKernelTelemetry {
	return &rawKernelTelemetry{
		Name_exceeds_max_size: t.Name_exceeds_max_size - other.Name_exceeds_max_size,
		Name_size_buckets:     computePathSizeBucketDifferences(t.Name_size_buckets, other.Name_size_buckets),
	}
}

func computePathSizeBucketDifferences(pathSizeBucket, otherPathSizeBucket [8]uint64) [8]uint64 {
	var result [8]uint64

	for i := 0; i < 8; i++ {
		result[i] = pathSizeBucket[i] - otherPathSizeBucket[i]
	}

	return result
}
