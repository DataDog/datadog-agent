// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

const (
	// MetricPrefix is the prefix of the metrics sent by the runtime security agent
	MetricPrefix = "datadog.runtime_security"

	// Perf buffer metrics

	// MetricPerfBufferInQueue is the name of the metric used to count the number of events currently in a perf buffer
	MetricPerfBufferInQueue = MetricPrefix + ".perf_buffer.in_queue"
	// MetricPerfBufferUsage is the name of the metric used to report the usage of a perf buffer
	MetricPerfBufferUsage = MetricPrefix + ".perf_buffer.usage"

	// MetricPerfBufferLostWrite is the name of the metric used to count the number of lost events, as reported by a
	// dedicated count in kernel space
	MetricPerfBufferLostWrite = MetricPrefix + ".perf_buffer.lost_events.write"
	// MetricPerfBufferLostRead is the name of the metric used to count the number of lost events, as reported in user
	// space by a perf buffer
	MetricPerfBufferLostRead = MetricPrefix + ".perf_buffer.lost_events.read"

	// MetricPerfBufferEventsWrite is the name of the metric used to count the number of events written to a perf buffer
	MetricPerfBufferEventsWrite = MetricPrefix + ".perf_buffer.events.write"
	// MetricPerfBufferEventsRead is the name of the metric used to count the number of events read from a perf buffer
	MetricPerfBufferEventsRead = MetricPrefix + ".perf_buffer.events.read"

	// MetricPerfBufferBytesWrite is the name of the metric used to count the number of bytes written to a perf buffer
	MetricPerfBufferBytesWrite = MetricPrefix + ".perf_buffer.bytes.write"
	// MetricPerfBufferBytesRead is the name of the metric used to count the number of bytes read from a perf buffer
	MetricPerfBufferBytesRead = MetricPrefix + ".perf_buffer.bytes.read"

	// MetricPrefixProcessResolverCacheSize is the name of the metric used to report the size of the user space
	// process cache
	MetricPrefixProcessResolverCacheSize = MetricPrefix+".process_resolver.cache_size"
)
