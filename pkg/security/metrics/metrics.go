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

	// MetricDiscarderAdded is the number of discarder added
	// Tags: discarder_type, event_type
	MetricDiscarderAdded = newRuntimeMetric(".discarders.discarder_added")
	// MetricEventDiscarded is the number of event discarded
	// Tags: discarder_type, event_type
	MetricEventDiscarded = newRuntimeMetric(".discarders.event_discarded")

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

	// Activity dump metrics

	// MetricActivityDumpEventProcessed is the name of the metric used to count the number of events processed while
	// creating an activity dump.
	// Tags: event_type
	MetricActivityDumpEventProcessed = newRuntimeMetric(".activity_dump.event.processed")
	// MetricActivityDumpEventAdded is the name of the metric used to count the number of events that were added to an
	// activity dump.
	// Tags: event_type
	MetricActivityDumpEventAdded = newRuntimeMetric(".activity_dump.event.added")
	// MetricActivityDumpSizeInBytes is the name of the metric used to report the size of the generated activity dumps in
	// bytes
	// Tags: format
	MetricActivityDumpSizeInBytes = newRuntimeMetric(".activity_dump.size_in_bytes")
	// MetricActivityDumpActiveDumps is the name of the metric used to report the number of active dumps
	// Tags: -
	MetricActivityDumpActiveDumps = newRuntimeMetric(".activity_dump.active_dumps")

	// Namespace resolver metrics

	// MetricNamespaceResolverNetNSHandle is the name of the metric used to report the count of netns handles
	// held by the NamespaceResolver.
	// Tags: -
	MetricNamespaceResolverNetNSHandle = newRuntimeMetric(".namespace_resolver.netns_handle")
	// MetricNamespaceResolverQueuedNetworkDevice is the name of the metric used to report the count of
	// queued network devices.
	// Tags: -
	MetricNamespaceResolverQueuedNetworkDevice = newRuntimeMetric(".namespace_resolver.queued_network_device")
	// MetricNamespaceResolverLonelyNetworkNamespace is the name of the metric used to report the count of
	// lonely network namespaces.
	// Tags: -
	MetricNamespaceResolverLonelyNetworkNamespace = newRuntimeMetric(".namespace_resolver.lonely_netns")

	// Others

	// MetricRuleSetLoaded is the name of the metric used to report that a new ruleset was loaded
	// Tags: -
	MetricRuleSetLoaded = newRuntimeMetric(".ruleset_loaded")
	// MetricSelfTest is the name of the metric used to report that a new ruleset was loaded
	// Tags: - success, fails
	MetricSelfTest = newRuntimeMetric(".self_test")
	// MetricTCProgram is the name of the metric used to report the count of active TC programs
	// Tags: -
	MetricTCProgram = newRuntimeMetric(".tc_program")

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

	// Runtime Compiled Constants metrics

	// MetricRuntimeCompiledConstantsEnabled is used to report if the runtime compilation has succeeded
	MetricRuntimeCompiledConstantsEnabled = newRuntimeCompiledConstantsMetric(".enabled")
	// MetricRuntimeCompiledConstantsCompilationResult is used to report the result of the runtime compilation
	MetricRuntimeCompiledConstantsCompilationResult = newRuntimeCompiledConstantsMetric(".compilation_result")
	// MetricRuntimeCompiledConstantsCompilationDuration is used to report the duration of the runtime compilation
	MetricRuntimeCompiledConstantsCompilationDuration = newRuntimeCompiledConstantsMetric(".compilation_duration")
	// MetricRuntimeCompiledConstantsHeaderFetchResult is used to report the result of the header fetching
	MetricRuntimeCompiledConstantsHeaderFetchResult = newRuntimeCompiledConstantsMetric(".header_fetch_result")

	// RuntimeMonitor metrics

	// MetricRuntimeMonitorGoAlloc is the name of the metric used to report the size in bytes of allocated heap objects
	// Tags: -
	MetricRuntimeMonitorGoAlloc = newRuntimeMetric(".runtime_monitor.go.alloc")
	// MetricRuntimeMonitorGoTotalAlloc is the name of the metric used to report the cumulative size of bytes allocated
	// for heap objects
	// Tags: -
	MetricRuntimeMonitorGoTotalAlloc = newRuntimeMetric(".runtime_monitor.go.total_alloc")
	// MetricRuntimeMonitorGoSys is the name of the metric used to report the total size in bytes of memory obtained from
	// the OS
	// Tags: -
	MetricRuntimeMonitorGoSys = newRuntimeMetric(".runtime_monitor.go.sys")
	// MetricRuntimeMonitorGoLookups is the name of the metric used to report the number of pointer lookups performed by
	// the runtime
	// Tags: -
	MetricRuntimeMonitorGoLookups = newRuntimeMetric(".runtime_monitor.go.lookups")
	// MetricRuntimeMonitorGoMallocs is the name of the metric used to report the cumulative count of allocated heap
	// objects
	// Tags: -
	MetricRuntimeMonitorGoMallocs = newRuntimeMetric(".runtime_monitor.go.mallocs")
	// MetricRuntimeMonitorGoFrees is the name of the metric used to report the cumulative count of freed heap objects
	// Tags: -
	MetricRuntimeMonitorGoFrees = newRuntimeMetric(".runtime_monitor.go.frees")
	// MetricRuntimeMonitorGoHeapAlloc is the name of the metric used to report the size in bytes of allocated heap
	// objects (including reachable and unreachable objects that the garbage collector has not yet freed)
	// Tags: -
	MetricRuntimeMonitorGoHeapAlloc = newRuntimeMetric(".runtime_monitor.go.heap_alloc")
	// MetricRuntimeMonitorGoHeapSys is the name of the metric used to report the size in bytes of heap memory obtained
	// from the OS. This includes virtual address space that has been reserved but not yet used, as well as virtual
	// address space for which the physical memory has been returned to the OS after it became unused
	// Tags: -
	MetricRuntimeMonitorGoHeapSys = newRuntimeMetric(".runtime_monitor.go.heap_sys")
	// MetricRuntimeMonitorGoHeapIdle is the name of the metric used to report the size in bytes in idle (unused) spans
	// Tags: -
	MetricRuntimeMonitorGoHeapIdle = newRuntimeMetric(".runtime_monitor.go.heap_idle")
	// MetricRuntimeMonitorGoHeapInuse is the name of the metric used to report the size in bytes in in-use spans
	// Tags: -
	MetricRuntimeMonitorGoHeapInuse = newRuntimeMetric(".runtime_monitor.go.heap_inuse")
	// MetricRuntimeMonitorGoHeapReleased is the name of the metric used to report the size in bytes of physical memory
	// returned to the OS
	// Tags: -
	MetricRuntimeMonitorGoHeapReleased = newRuntimeMetric(".runtime_monitor.go.heap_released")
	// MetricRuntimeMonitorGoHeapObjects is the name of the metric used to report the number of allocated heap objects
	// Tags: -
	MetricRuntimeMonitorGoHeapObjects = newRuntimeMetric(".runtime_monitor.go.heap_objects")
	// MetricRuntimeMonitorGoStackInuse is the name of the metric used to report the size in bytes of stack spans
	// Tags: -
	MetricRuntimeMonitorGoStackInuse = newRuntimeMetric(".runtime_monitor.go.stack_inuse")
	// MetricRuntimeMonitorGoStackSys is the name of the metric used to report the size in bytes of stack memory obtained
	// from the OS
	// Tags: -
	MetricRuntimeMonitorGoStackSys = newRuntimeMetric(".runtime_monitor.go.stack_sys")
	// MetricRuntimeMonitorGoMSpanInuse is the name of the metric used to report the size in bytes of allocated mspan
	// structures
	// Tags: -
	MetricRuntimeMonitorGoMSpanInuse = newRuntimeMetric(".runtime_monitor.go.mspan_inuse")
	// MetricRuntimeMonitorGoMSpanSys is the name of the metric used to report the size in bytes of memory obtained from
	// the OS for mspan structures
	// Tags: -
	MetricRuntimeMonitorGoMSpanSys = newRuntimeMetric(".runtime_monitor.go.mspan_sys")
	// MetricRuntimeMonitorGoMCacheInuse is the name of the metric used to report the size in bytes of allocated mcache
	// structures
	// Tags: -
	MetricRuntimeMonitorGoMCacheInuse = newRuntimeMetric(".runtime_monitor.go.mcache_inuse")
	// MetricRuntimeMonitorGoMCacheSys is the name of the metric used to report the size in bytes of memory obtained from
	// the OS for mcache structures
	// Tags: -
	MetricRuntimeMonitorGoMCacheSys = newRuntimeMetric(".runtime_monitor.go.mcache_sys")
	// MetricRuntimeMonitorGoBuckHashSys is the name of the metric used to report the size in bytes of memory in profiling
	// bucket hash tables
	// Tags: -
	MetricRuntimeMonitorGoBuckHashSys = newRuntimeMetric(".runtime_monitor.go.buck_hash_sys")
	// MetricRuntimeMonitorGoGCSys is the name of the metric used to report the size in bytes of memory in garbage
	// collection metadata
	// Tags: -
	MetricRuntimeMonitorGoGCSys = newRuntimeMetric(".runtime_monitor.go.gc_sys")
	// MetricRuntimeMonitorGoOtherSys is the name of the metric used to report the size in bytes of memory in miscellaneous
	// off-heap runtime allocations
	// Tags: -
	MetricRuntimeMonitorGoOtherSys = newRuntimeMetric(".runtime_monitor.go.other_sys")
	// MetricRuntimeMonitorGoNextGC is the name of the metric used to report the target heap size of the next GC cycle
	// Tags: -
	MetricRuntimeMonitorGoNextGC = newRuntimeMetric(".runtime_monitor.go.next_gc")
	// MetricRuntimeMonitorGoNumGC is the name of the metric used to report the number of completed GC cycles
	// Tags: -
	MetricRuntimeMonitorGoNumGC = newRuntimeMetric(".runtime_monitor.go.num_gc")
	// MetricRuntimeMonitorGoNumForcedGC is the name of the metric used to report the number of GC cycles that were forced
	// by the application calling the GC function
	// Tags: -
	MetricRuntimeMonitorGoNumForcedGC = newRuntimeMetric(".runtime_monitor.go.num_forced_gc")

	// MetricRuntimeMonitorProcRSS is the name of the metric used to report the RSS in bytes retrieved from Procfs
	// Tags: -
	MetricRuntimeMonitorProcRSS = newRuntimeMetric(".runtime_monitor.proc.rss")
	// MetricRuntimeMonitorProcVMS is the name of the metric used to report the VMS in bytes retrieved from Procfs
	// Tags: -
	MetricRuntimeMonitorProcVMS = newRuntimeMetric(".runtime_monitor.proc.vms")
	// MetricRuntimeMonitorProcShared is the name of the metric used to report the shared memory in bytes retrieved from Procfs
	// Tags: -
	MetricRuntimeMonitorProcShared = newRuntimeMetric(".runtime_monitor.proc.shared")
	// MetricRuntimeMonitorProcText is the name of the metric used to report the text memory in bytes retrieved from Procfs
	// Tags: -
	MetricRuntimeMonitorProcText = newRuntimeMetric(".runtime_monitor.proc.text")
	// MetricRuntimeMonitorProcLib is the name of the metric used to report the lib memory in bytes retrieved from Procfs
	// Tags: -
	MetricRuntimeMonitorProcLib = newRuntimeMetric(".runtime_monitor.proc.lib")
	// MetricRuntimeMonitorProcData is the name of the metric used to report the data memory in bytes retrieved from Procfs
	// Tags: -
	MetricRuntimeMonitorProcData = newRuntimeMetric(".runtime_monitor.proc.data")
	// MetricRuntimeMonitorProcDirty is the name of the metric used to report the dirty memory in bytes retrieved from Procfs
	// Tags: -
	MetricRuntimeMonitorProcDirty = newRuntimeMetric(".runtime_monitor.proc.dirty")

	// MetricRuntimeCgroupMemoryStatPrefix is the prefix for the metrics collected in the memory.stat cgroup file
	// Tags: -
	MetricRuntimeCgroupMemoryStatPrefix = newRuntimeMetric(".runtime_monitor.cgroup.memory_stat.")
	// MetricRuntimeCgroupMemoryUsageInBytes is the name of the metric used to report memory.usage_in_bytes
	// Tags: -
	MetricRuntimeCgroupMemoryUsageInBytes = newRuntimeMetric(".runtime_monitor.cgroup.memory.usage_in_bytes")
	// MetricRuntimeCgroupMemoryLimitInBytes is the name of the metric used to report memory.limit_in_bytes
	// Tags: -
	MetricRuntimeCgroupMemoryLimitInBytes = newRuntimeMetric(".runtime_monitor.cgroup.memory.limit_in_bytes")
	// MetricRuntimeCgroupMemoryMemSWUsageInBytes is the name of the metric used to report memory.memsw.usage_in_bytes
	// Tags: -
	MetricRuntimeCgroupMemoryMemSWUsageInBytes = newRuntimeMetric(".runtime_monitor.cgroup.memory.memsw_usage_in_bytes")
	// MetricRuntimeCgroupMemoryMemSWLimitInBytes is the name of the metric used to report memory.memsw.limit_in_bytes
	// Tags: -
	MetricRuntimeCgroupMemoryMemSWLimitInBytes = newRuntimeMetric(".runtime_monitor.cgroup.memory.memsw_limit_in_bytes")
	// MetricRuntimeCgroupMemoryKmemUsageInBytes is the name of the metric used to report memory.kmem.usage_in_bytes
	// Tags: -
	MetricRuntimeCgroupMemoryKmemUsageInBytes = newRuntimeMetric(".runtime_monitor.cgroup.memory.kmem_usage_in_bytes")
	// MetricRuntimeCgroupMemoryKmemLimitInBytes is the name of the metric used to report memory.kmem.limit_in_bytes
	// Tags: -
	MetricRuntimeCgroupMemoryKmemLimitInBytes = newRuntimeMetric(".runtime_monitor.cgroup.memory.kmem_limit_in_bytes")
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
