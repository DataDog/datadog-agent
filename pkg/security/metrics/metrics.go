// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

var (
	// MetricRuntimePrefix is the prefix of the metrics sent by the runtime security module
	MetricRuntimePrefix = "datadog.runtime_security"

	// MetricAgentPrefix is the prefix of the metrics sent by the runtime security agent
	MetricAgentPrefix = "datadog.security_agent"

	// Event server

	// MetricEventServerExpired is the name of the metric used to count the number of events that expired because the
	// security-agent was not processing them fast enough
	// Tags: rule_id
	MetricEventServerExpired = newRuntimeMetric(".rules.event_server.expired")

	// Load controller metrics

	// MetricLoadControllerPidDiscarder is the name of the metric used to count the number of pid discarders
	// Tags: event_type
	MetricLoadControllerPidDiscarder = newRuntimeMetric(".load_controller.pids_discarder")

	// Rate limiter metrics

	// MetricRateLimiterDrop is the name of the metric used to count the amount of events dropped by the rate limiter
	// Tags: rule_id
	MetricRateLimiterDrop = newRuntimeMetric(".rules.rate_limiter.drop")
	// MetricRateLimiterAllow is the name of the metric used to count the amount of events allowed by the rate limiter
	// Tags: rule_id
	MetricRateLimiterAllow = newRuntimeMetric(".rules.rate_limiter.allow")

	// Syscall monitoring metrics

	// MetricSyscalls is the name of the metric used to count each syscall executed on the host
	// Tags: process, syscall
	MetricSyscalls = newRuntimeMetric(".syscalls")
	// MetricExec is the name of the metric used to count the executions on the host
	// Tags: process
	MetricExec = newRuntimeMetric(".exec")
	// MetricConcurrentSyscall is the name of the metric used to count concurrent syscalls
	// Tags: -
	MetricConcurrentSyscall = newRuntimeMetric(".concurrent_syscalls")

	// Perf buffer metrics

	// MetricPerfBufferLostWrite is the name of the metric used to count the number of lost events, as reported by a
	// dedicated count in kernel space
	// Tags: map, cpu, event_type
	MetricPerfBufferLostWrite = newRuntimeMetric(".perf_buffer.lost_events.write")
	// MetricPerfBufferLostRead is the name of the metric used to count the number of lost events, as reported in user
	// space by a perf buffer
	// Tags: map, cpu
	MetricPerfBufferLostRead = newRuntimeMetric(".perf_buffer.lost_events.read")

	// MetricPerfBufferEventsWrite is the name of the metric used to count the number of events written to a perf buffer
	// Tags: map, cpu, event_type
	MetricPerfBufferEventsWrite = newRuntimeMetric(".perf_buffer.events.write")
	// MetricPerfBufferEventsRead is the name of the metric used to count the number of events read from a perf buffer
	// Tags: map, cpu
	MetricPerfBufferEventsRead = newRuntimeMetric(".perf_buffer.events.read")

	// MetricPerfBufferBytesWrite is the name of the metric used to count the number of bytes written to a perf buffer
	// Tags: map, cpu, event_type
	MetricPerfBufferBytesWrite = newRuntimeMetric(".perf_buffer.bytes.write")
	// MetricPerfBufferBytesRead is the name of the metric used to count the number of bytes read from a perf buffer
	// Tags: map, cpu
	MetricPerfBufferBytesRead = newRuntimeMetric(".perf_buffer.bytes.read")
	// MetricPerfBufferSortingError is the name of the metric used to report events reordering issues.
	// Tags: map, cpu, event_type
	MetricPerfBufferSortingError = newRuntimeMetric(".perf_buffer.sorting_error")
	// MetricPerfBufferSortingQueueSize is the name of the metric used to report reordering queue size.
	MetricPerfBufferSortingQueueSize = newRuntimeMetric(".perf_buffer.sorting_queue_size")
	// MetricPerfBufferSortingAvgOp is the name of the metric used to report average sorting operations.
	MetricPerfBufferSortingAvgOp = newRuntimeMetric(".perf_buffer.sorting_avg_op")

	// Process Resolver metrics

	// MetricProcessResolverCacheSize is the name of the metric used to report the size of the user space
	// process cache
	// Tags: -
	MetricProcessResolverCacheSize = newRuntimeMetric(".process_resolver.cache_size")
	// MetricProcessResolverReferenceCount is the name of the metric used to report the number of entry cache still
	// referenced in the process tree
	// Tags: -
	MetricProcessResolverReferenceCount = newRuntimeMetric(".process_resolver.reference_count")
	// MetricProcessResolverCacheMiss is the name of the metric used to report process resolver cache misses
	// Tags: -
	MetricProcessResolverCacheMiss = newRuntimeMetric(".process_resolver.cache_miss")
	// MetricProcessResolverCacheHits is the name of the metric used to report the process resolver cache hits
	// Tags: type
	MetricProcessResolverCacheHits = newRuntimeMetric(".process_resolver.hits")
	// MetricProcessResolverAdded is the name of the metric used to report the number of entries added in the cache
	// Tags: -
	MetricProcessResolverAdded = newRuntimeMetric(".process_resolver.added")
	// MetricProcessResolverFlushed is the name of the metric used to report the number cache flush
	// Tags: -
	MetricProcessResolverFlushed = newRuntimeMetric(".process_resolver.flushed")

	// Custom events

	// MetricRuleSetLoaded is the name of the metric used to report that a new ruleset was loaded
	// Tags: -
	MetricRuleSetLoaded = newRuntimeMetric(".ruleset_loaded")
	// MetricForkBomb is the name of the metric used to report the number of processes that crossed the fork bomb
	// threshold. Tags: -
	MetricForkBomb = newRuntimeMetric(".fork_bomb")

	// Security Agent metrics

	// MetricsSecurityAgentRuntimeRunning is reported when the security agent `Runtime` feature is enabled
	MetricsSecurityAgentRuntimeRunning = newAgentMetric(".runtime.running")
	// MetricsSecurityAgentFIMRunning is reported when the security agent `FIM` feature is enabled
	MetricsSecurityAgentFIMRunning = newAgentMetric(".fim.running")
)

func newRuntimeMetric(name string) string {
	return MetricRuntimePrefix + name
}

func newAgentMetric(name string) string {
	return MetricAgentPrefix + name
}
