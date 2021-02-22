// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

var (
	// MetricPrefix is the prefix of the metrics sent by the runtime security agent
	MetricPrefix = "datadog.runtime_security"

	// Event server

	// MetricEventServerExpired is the name of the metric used to count the number of events that expired because the
	// security-agent was not processing them fast enough
	// Tags: rule_id
	MetricEventServerExpired = newRuntimeSecurityMetric(".rules.event_server.expired")

	// Load controller metrics

	// MetricLoadControllerPidDiscarder is the name of the metric used to count the number of pid discarders
	// Tags: event_type
	MetricLoadControllerPidDiscarder = newRuntimeSecurityMetric(".load_controller.pids_discarder")

	// Rate limiter metrics

	// MetricRateLimiterDrop is the name of the metric used to count the amount of events dropped by the rate limiter
	// Tags: rule_id
	MetricRateLimiterDrop = newRuntimeSecurityMetric(".rules.rate_limiter.drop")
	// MetricRateLimiterAllow is the name of the metric used to count the amount of events allowed by the rate limiter
	// Tags: rule_id
	MetricRateLimiterAllow = newRuntimeSecurityMetric(".rules.rate_limiter.allow")

	// Syscall monitoring metrics

	// MetricSyscalls is the name of the metric used to count each syscall executed on the host
	// Tags: process, syscall
	MetricSyscalls = newRuntimeSecurityMetric(".syscalls")
	// MetricExec is the name of the metric used to count the executions on the host
	// Tags: process
	MetricExec = newRuntimeSecurityMetric(".exec")
	// MetricConcurrentSyscall is the name of the metric used to count concurrent syscalls
	// Tags: -
	MetricConcurrentSyscall = newRuntimeSecurityMetric(".concurrent_syscalls")

	// Perf buffer metrics

	// MetricPerfBufferLostWrite is the name of the metric used to count the number of lost events, as reported by a
	// dedicated count in kernel space
	// Tags: map, cpu, event_type
	MetricPerfBufferLostWrite = newRuntimeSecurityMetric(".perf_buffer.lost_events.write")
	// MetricPerfBufferLostRead is the name of the metric used to count the number of lost events, as reported in user
	// space by a perf buffer
	// Tags: map, cpu
	MetricPerfBufferLostRead = newRuntimeSecurityMetric(".perf_buffer.lost_events.read")

	// MetricPerfBufferEventsWrite is the name of the metric used to count the number of events written to a perf buffer
	// Tags: map, cpu, event_type
	MetricPerfBufferEventsWrite = newRuntimeSecurityMetric(".perf_buffer.events.write")
	// MetricPerfBufferEventsRead is the name of the metric used to count the number of events read from a perf buffer
	// Tags: map, cpu
	MetricPerfBufferEventsRead = newRuntimeSecurityMetric(".perf_buffer.events.read")

	// MetricPerfBufferBytesWrite is the name of the metric used to count the number of bytes written to a perf buffer
	// Tags: map, cpu, event_type
	MetricPerfBufferBytesWrite = newRuntimeSecurityMetric(".perf_buffer.bytes.write")
	// MetricPerfBufferBytesRead is the name of the metric used to count the number of bytes read from a perf buffer
	// Tags: map, cpu
	MetricPerfBufferBytesRead = newRuntimeSecurityMetric(".perf_buffer.bytes.read")
	// MetricPerfBufferSortingError is the name of the metric used to report events reordering issues.
	// Tags: map, cpu, event_type
	MetricPerfBufferSortingError = newRuntimeSecurityMetric(".perf_buffer.sorting_error")
	// MetricPerfBufferSortingQueueSize is the name of the metric used to report reordering queue size.
	MetricPerfBufferSortingQueueSize = newRuntimeSecurityMetric(".perf_buffer.sorting_queue_size")
	// MetricPerfBufferSortingAvgOp is the name of the metric used to report average sorting operations.
	MetricPerfBufferSortingAvgOp = newRuntimeSecurityMetric(".perf_buffer.sorting_avg_op")

	// Process Resolver metrics

	// MetricProcessResolverCacheSize is the name of the metric used to report the size of the user space
	// process cache
	// Tags: -
	MetricProcessResolverCacheSize = newRuntimeSecurityMetric(".process_resolver.cache_size")
	// MetricProcessResolverReferenceCount is the name of the metric used to report the number of entry cache still
	// referenced in the process tree
	// Tags: -
	MetricProcessResolverReferenceCount = newRuntimeSecurityMetric(".process_resolver.reference_count")
	// MetricProcessResolverCacheMiss is the name of the metric used to report process resolver cache misses
	// Tags: -
	MetricProcessResolverCacheMiss = newRuntimeSecurityMetric(".process_resolver.cache_miss")
	// MetricProcessResolverCacheHits is the name of the metric used to report the process resolver cache hits
	// Tags: type
	MetricProcessResolverCacheHits = newRuntimeSecurityMetric(".process_resolver.hits")
	// MetricProcessResolverAdded is the name of the metric used to report the number of entries added in the cache
	// Tags: -
	MetricProcessResolverAdded = newRuntimeSecurityMetric(".process_resolver.added")
	// MetricProcessResolverFlushed is the name of the metric used to report the number cache flush
	// Tags: -
	MetricProcessResolverFlushed = newRuntimeSecurityMetric(".process_resolver.flushed")

	// Custom events

	// MetricRuleSetLoaded is the name of the metric used to report that a new ruleset was loaded
	// Tags: -
	MetricRuleSetLoaded = newRuntimeSecurityMetric(".ruleset_loaded")
	// MetricForkBomb is the name of the metric used to report the number of processes that crossed the fork bomb
	// threshold. Tags: -
	MetricForkBomb = newRuntimeSecurityMetric(".fork_bomb")
)

func newRuntimeSecurityMetric(name string) string {
	return MetricPrefix + name
}
