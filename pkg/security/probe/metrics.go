// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	MetricRateLimiterAllow = newRuntimeSecurityMetric(".rules.rate_limiter.drop")

	// Syscall monitoring metrics

	// MetricSyscalls is the name of the metric used to count each syscall executed on the host
	MetricSyscalls = newRuntimeSecurityMetric(".syscalls")
	// MetricExec is the name of the metric used to count the executions on the host
	MetricExec = newRuntimeSecurityMetric(".exec")
	// MetricConcurrentSyscall is the name of the metric used to count concurrent syscalls
	MetricConcurrentSyscall = newRuntimeSecurityMetric(".concurrent_syscalls")

	// Perf buffer metrics

	// MetricPerfBufferInQueue is the name of the metric used to count the number of events currently in a perf buffer
	MetricPerfBufferInQueue = newRuntimeSecurityMetric(".perf_buffer.in_queue")
	// MetricPerfBufferUsage is the name of the metric used to report the usage of a perf buffer
	MetricPerfBufferUsage = newRuntimeSecurityMetric(".perf_buffer.usage")

	// MetricPerfBufferLostWrite is the name of the metric used to count the number of lost events, as reported by a
	// dedicated count in kernel space
	MetricPerfBufferLostWrite = newRuntimeSecurityMetric(".perf_buffer.lost_events.write")
	// MetricPerfBufferLostRead is the name of the metric used to count the number of lost events, as reported in user
	// space by a perf buffer
	MetricPerfBufferLostRead = newRuntimeSecurityMetric(".perf_buffer.lost_events.read")

	// MetricPerfBufferEventsWrite is the name of the metric used to count the number of events written to a perf buffer
	MetricPerfBufferEventsWrite = newRuntimeSecurityMetric(".perf_buffer.events.write")
	// MetricPerfBufferEventsRead is the name of the metric used to count the number of events read from a perf buffer
	MetricPerfBufferEventsRead = newRuntimeSecurityMetric(".perf_buffer.events.read")

	// MetricPerfBufferBytesWrite is the name of the metric used to count the number of bytes written to a perf buffer
	MetricPerfBufferBytesWrite = newRuntimeSecurityMetric(".perf_buffer.bytes.write")
	// MetricPerfBufferBytesRead is the name of the metric used to count the number of bytes read from a perf buffer
	MetricPerfBufferBytesRead = newRuntimeSecurityMetric(".perf_buffer.bytes.read")

	// Process Resolver metrics

	// MetricPrefixProcessResolverCacheSize is the name of the metric used to report the size of the user space
	// process cache
	MetricPrefixProcessResolverCacheSize = newRuntimeSecurityMetric(".process_resolver.cache_size")

	// Custom events

	// MetricRuleSetLoaded is the name of the metric used to report that a new ruleset was loaded
	MetricRuleSetLoaded = newRuntimeSecurityMetric(".ruleset_loaded")
)

func newRuntimeSecurityMetric(name string) string {
	return MetricPrefix + name
}
