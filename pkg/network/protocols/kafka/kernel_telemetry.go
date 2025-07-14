// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"strconv"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

type kernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// pathSizeBucket Count of topic names sizes divided into buckets.
	pathSizeBucket [TopicNameBuckets]*libtelemetry.Counter

	// produceNoRequiredAcks is the number of produce requests that did not require any acks.
	produceNoRequiredAcks *libtelemetry.Counter

	// telemetryLastState represents the latest Kafka eBPF Kernel telemetry observed from the kernel
	telemetryLastState RawKernelTelemetry
}

// newKernelTelemetry hold Kafka kernel metrics.
func newKernelTelemetry() *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.kafka", libtelemetry.OptPrometheus)
	kafkaKernelTel := &kernelTelemetry{
		metricGroup: metricGroup}

	for bucketIndex := range kafkaKernelTel.pathSizeBucket {
		kafkaKernelTel.pathSizeBucket[bucketIndex] = metricGroup.NewCounter("path_size_bucket_" + (strconv.Itoa(bucketIndex + 1)))
	}

	kafkaKernelTel.produceNoRequiredAcks = metricGroup.NewCounter("produce_no_required_acks")

	return kafkaKernelTel
}

// update updates the kernel metrics with the given telemetry.
func (t *kernelTelemetry) update(tel *RawKernelTelemetry) {
	// We should only add the delta between the current eBPF map state and the last seen eBPF map state
	telemetryDelta := tel.Sub(t.telemetryLastState)
	for bucketIndex := range t.pathSizeBucket {
		t.pathSizeBucket[bucketIndex].Add(int64(telemetryDelta.Topic_name_size_buckets[bucketIndex]))
	}
	t.produceNoRequiredAcks.Add(int64(telemetryDelta.Produce_no_required_acks))

	// Create a deep copy of the 'tel' parameter to prevent changes from the outer scope affecting the last state
	t.telemetryLastState = *tel
}

// Sub generates a new RawKernelTelemetry object by subtracting the values of this RawKernelTelemetry object from the other
func (t *RawKernelTelemetry) Sub(other RawKernelTelemetry) *RawKernelTelemetry {
	return &RawKernelTelemetry{
		Topic_name_size_buckets:  computePathSizeBucketDifferences(t.Topic_name_size_buckets, other.Topic_name_size_buckets),
		Produce_no_required_acks: t.Produce_no_required_acks - other.Produce_no_required_acks,
	}
}

func computePathSizeBucketDifferences(pathSizeBucket, otherPathSizeBucket [TopicNameBuckets]uint64) [TopicNameBuckets]uint64 {
	var result [TopicNameBuckets]uint64

	for i := 0; i < TopicNameBuckets; i++ {
		result[i] = pathSizeBucket[i] - otherPathSizeBucket[i]
	}

	return result
}
