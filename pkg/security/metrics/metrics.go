// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

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
	// Tags: -
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

	// Dentry Resolver metrics

	// MetricDentryResolverHits is the counter of successful dentry resolution
	// Tags: cache, kernel_maps
	MetricDentryResolverHits = newRuntimeMetric(".dentry_resolver.hits")
	// MetricDentryResolverMiss is the counter of unsuccessful dentry resolution
	// Tags: cache, kernel_maps
	MetricDentryResolverMiss = newRuntimeMetric(".dentry_resolver.miss")
	// MetricDentryERPC is the counter of eRPC dentry resolution errors by error type
	// Tags: ret
	MetricDentryERPC = newRuntimeMetric(".dentry_resolver.erpc")

	// filtering metrics

	// MetricInodeDiscardersAdded is the number of inode discarder added
	MetricInodeDiscardersAdded = newRuntimeMetric(".discarders.inode.added")

	// Perf buffer metrics

	// MetricPerfBufferLostWrite is the name of the metric used to count the number of lost events, as reported by a
	// dedicated count in kernel space
	// Tags: map, event_type
	MetricPerfBufferLostWrite = newRuntimeMetric(".perf_buffer.lost_events.write")
	// MetricPerfBufferLostRead is the name of the metric used to count the number of lost events, as reported in user
	// space by a perf buffer
	// Tags: map
	MetricPerfBufferLostRead = newRuntimeMetric(".perf_buffer.lost_events.read")

	// MetricPerfBufferEventsWrite is the name of the metric used to count the number of events written to a perf buffer
	// Tags: map, event_type
	MetricPerfBufferEventsWrite = newRuntimeMetric(".perf_buffer.events.write")
	// MetricPerfBufferEventsRead is the name of the metric used to count the number of events read from a perf buffer
	// Tags: map
	MetricPerfBufferEventsRead = newRuntimeMetric(".perf_buffer.events.read")

	// MetricPerfBufferBytesWrite is the name of the metric used to count the number of bytes written to a perf buffer
	// Tags: map, event_type
	MetricPerfBufferBytesWrite = newRuntimeMetric(".perf_buffer.bytes.write")
	// MetricPerfBufferBytesRead is the name of the metric used to count the number of bytes read from a perf buffer
	// Tags: map
	MetricPerfBufferBytesRead = newRuntimeMetric(".perf_buffer.bytes.read")
	// MetricPerfBufferSortingError is the name of the metric used to report events reordering issues.
	// Tags: map, event_type
	MetricPerfBufferSortingError = newRuntimeMetric(".perf_buffer.sorting_error")
	// MetricPerfBufferSortingQueueSize is the name of the metric used to report reordering queue size.
	// Tags: -
	MetricPerfBufferSortingQueueSize = newRuntimeMetric(".perf_buffer.sorting_queue_size")
	// MetricPerfBufferSortingAvgOp is the name of the metric used to report average sorting operations.
	// Tags: -
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

	// Security Agent metrics

	// MetricSecurityAgentRuntimeRunning is reported when the security agent `Runtime` feature is enabled
	MetricSecurityAgentRuntimeRunning = newAgentMetric(".runtime.running")
	// MetricSecurityAgentFIMRunning is reported when the security agent `FIM` feature is enabled
	MetricSecurityAgentFIMRunning = newAgentMetric(".fim.running")

	// MetricSecurityAgentRuntimeContainersRunning is used to report the count of running containers when the security agent
	// `Runtime` feature is enabled
	MetricSecurityAgentRuntimeContainersRunning = newAgentMetric(".runtime.containers_running")
	// MetricSecurityAgentFIMContainersRunning is used to report the count of running containers when the security agent
	// `FIM` feature is enabled
	MetricSecurityAgentFIMContainersRunning = newAgentMetric(".fim.containers_running")

	// Runtime Compilaed Constants metrics

	// MetricRuntimeCompiledConstantsEnabled is used to report if the runtime compilation has succeeded
	MetricRuntimeCompiledConstantsEnabled = newRuntimeCompiledConstantsMetric(".enabled")
	// MetricRuntimeCompiledConstantsCompilationResult is used to report the result of the runtime compilation
	MetricRuntimeCompiledConstantsCompilationResult = newRuntimeCompiledConstantsMetric(".compilation_result")
	// MetricRuntimeCompiledConstantsCompilationDuration is used to report the duration of the runtime compilation
	MetricRuntimeCompiledConstantsCompilationDuration = newRuntimeCompiledConstantsMetric(".compilation_duration")
	// MetricRuntimeCompiledConstantsHeaderFetchResult is used to report the result of the header fetching
	MetricRuntimeCompiledConstantsHeaderFetchResult = newRuntimeCompiledConstantsMetric(".header_fetch_result")
)

// SetTagsWithCardinality returns the array of tags and set the requested cardinality
func SetTagsWithCardinality(cardinality string, tags ...string) []string {
	return append(tags, fmt.Sprintf("%s:%s", dogstatsd.CardinalityTagPrefix, cardinality))
}

var (
	// CacheTag is assigned to metrics related to userspace cache
	CacheTag = "type:cache"
	// KernelMapsTag is assigned to metrics related to eBPF kernel maps
	KernelMapsTag = "type:kernel_maps"
	// ProcFSTag is assigned to metrics related to /proc fallbacks
	ProcFSTag = "type:procfs"
	// ERPCTag is assigned to metrics related to eRPC
	ERPCTag = "type:erpc"
	// AllTypesTags is the list of types
	AllTypesTags = []string{CacheTag, KernelMapsTag, ProcFSTag, ERPCTag}

	// SegmentResolutionTag is assigned to metrics related to the resolution of a segment
	SegmentResolutionTag = "resolution:segment"
	// ParentResolutionTag is assigned to metrics related to the resolution of a parent
	ParentResolutionTag = "resolution:parent"
	// PathResolutionTag is assigned to metrics related to the resolution of a path
	PathResolutionTag = "resolution:path"
	// AllResolutionsTags is the list of resolution tags
	AllResolutionsTags = []string{SegmentResolutionTag, ParentResolutionTag, PathResolutionTag}
)

func newRuntimeMetric(name string) string {
	return MetricRuntimePrefix + name
}

func newAgentMetric(name string) string {
	return MetricAgentPrefix + name
}

func newRuntimeCompiledConstantsMetric(name string) string {
	return newRuntimeMetric(".runtime_compilation.constants" + name)
}
