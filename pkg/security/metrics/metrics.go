// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"

	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
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
	// MetricProcessEventsServerExpired is the name of the metric used to count the number of process events that
	// expired because the process-agent was not processing them fast enough
	// Tags: -
	MetricProcessEventsServerExpired = newRuntimeMetric(".event_server.process_events_expired")

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
	// MetricPerfBufferBytesInUse is the name of the metric used to count the percentage of space left in the ring buffer
	// Tags: map
	MetricPerfBufferBytesInUse = newRuntimeMetric(".perf_buffer.bytes.in_use")
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
	// MetricProcessResolverMiss is the name of the metric used to report process resolver cache misses
	// Tags: -
	MetricProcessResolverMiss = newRuntimeMetric(".process_resolver.miss")
	// MetricProcessResolverPathError is the name of the metric used to report process path resolution errors
	// Tags: -
	MetricProcessResolverPathError = newRuntimeMetric(".process_resolver.path_error")
	// MetricProcessResolverHits is the name of the metric used to report the process resolver cache hits
	// Tags: type
	MetricProcessResolverHits = newRuntimeMetric(".process_resolver.hits")
	// MetricProcessResolverAdded is the name of the metric used to report the number of entries added in the cache
	// Tags: -
	MetricProcessResolverAdded = newRuntimeMetric(".process_resolver.added")
	// MetricProcessResolverFlushed is the name of the metric used to report the number cache flush
	// Tags: -
	MetricProcessResolverFlushed = newRuntimeMetric(".process_resolver.flushed")
	// MetricProcessResolverArgsTruncated is the name of the metric used to report the number of args truncated
	// Tags: -
	MetricProcessResolverArgsTruncated = newRuntimeMetric(".process_resolver.args.truncated")
	// MetricProcessResolverArgsSize is the name of the metric used to report the number of args size
	// Tags: -
	MetricProcessResolverArgsSize = newRuntimeMetric(".process_resolver.args.size")
	// MetricProcessResolverEnvsTruncated is the name of the metric used to report the number of envs truncated
	// Tags: -
	MetricProcessResolverEnvsTruncated = newRuntimeMetric(".process_resolver.envs.truncated")
	// MetricProcessResolverEnvsSize is the name of the metric used to report the number of envs size
	// Tags: -
	MetricProcessResolverEnvsSize = newRuntimeMetric(".process_resolver.envs.size")
	// MetricProcessEventBrokenLineage is the name of the metric used to report a broken lineage
	// Tags: -
	MetricProcessEventBrokenLineage = newRuntimeMetric(".process_resolver.event_broken_lineage")

	// Mount resolver metrics

	// MetricMountResolverCacheSize is the name of the metric used to report the size of the user space
	// mount cache
	// Tags: -
	MetricMountResolverCacheSize = newRuntimeMetric(".mount_resolver.cache_size")
	// MetricMountResolverHits is the counter of successful mount resolution
	// Tags: cache, procfs
	MetricMountResolverHits = newRuntimeMetric(".mount_resolver.hits")
	// MetricDentryResolverMiss is the counter of unsuccessful mount resolution
	// Tags: cache, procfs
	MetricMountResolverMiss = newRuntimeMetric(".mount_resolver.miss")

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
	// Tags: format, storage_type, compression
	MetricActivityDumpSizeInBytes = newRuntimeMetric(".activity_dump.size_in_bytes")
	// MetricActivityDumpActiveDumps is the name of the metric used to report the number of active dumps
	// Tags: -
	MetricActivityDumpActiveDumps = newRuntimeMetric(".activity_dump.active_dumps")
	// MetricActivityDumpPathMergeCount is the name of the metric used to report the number of path merged
	// Tags: -
	MetricActivityDumpPathMergeCount = newRuntimeMetric(".activity_dump.path_merged")
	// MetricActivityDumpLoadControllerTriggered is the name of the metric used to report that the ADM load controller reduced the config envelope
	// Tags:reduction, event_type
	MetricActivityDumpLoadControllerTriggered = newRuntimeMetric(".activity_dump.load_controller_triggered")
	// MetricActivityDumpActiveDumpSizeInMemory is the size of an activity dump in memory
	// Tags: dump_index
	MetricActivityDumpActiveDumpSizeInMemory = newRuntimeMetric(".activity_dump.size_in_memory")
	// MetricActivityDumpEntityTooLarge is the name of the metric used to report the number of active dumps that couldn't
	// be sent because they are too big
	// Tags: format, compression
	MetricActivityDumpEntityTooLarge = newAgentMetric(".activity_dump.entity_too_large")
	// MetricActivityDumpBrokenLineageDrop is the name of the metric used to report the number of events dropped due to broken ancestors lineage
	// Tags: -
	MetricActivityDumpBrokenLineageDrop = newRuntimeMetric(".activity_dump.broken_lineage_drop")
	// MetricActivityDumpEventTypeDrop is the name of the metric used to report the number of event dropped because their event types is not traced
	// Tags: event_type
	MetricActivityDumpEventTypeDrop = newRuntimeMetric(".activity_dump.event_type_drop")
	// MetricActivityDumpValidRootNodeDrop is the name of the metric used to report the number of dropped root not valide node
	// Tags: -
	MetricActivityDumpValidRootNodeDrop = newRuntimeMetric(".activity_dump.valid_root_node_drop")
	// MetricActivityDumpBindFamilyDrop is the name of the metric used to report the number of event dropped because the address family is not handled
	// Tags: -
	MetricActivityDumpBindFamilyDrop = newRuntimeMetric(".activity_dump.bind_family_drop")
	// MetricActivityDumpEmptyDropped is the name of the metric used to report the number of activity dumps dropped because they were empty
	// Tags: -
	MetricActivityDumpEmptyDropped = newRuntimeMetric(".activity_dump.empty_dump_dropped")
	// MetricActivityDumpDropMaxDumpReached is the name of the metric used to report that an activity dump was dropped because the maximum amount of dumps for a workload was reached
	// Tags: -
	MetricActivityDumpDropMaxDumpReached = newRuntimeMetric(".activity_dump.drop_max_dump_reached")
	// MetricActivityDumpNotYetProfiledWorkload is the name of the metric used to report the count of workload not yet profiled
	// Tags: -
	MetricActivityDumpNotYetProfiledWorkload = newAgentMetric(".activity_dump.not_yet_profiled_workload")

	// SBOM resolver metrics

	// MetricSBOMResolverActiveSBOMs is the name of the metric used to report the count of SBOMs kept in memory
	// Tags: -
	MetricSBOMResolverActiveSBOMs = newRuntimeMetric(".sbom_resolver.active_sboms")
	// MetricSBOMResolverSBOMGenerations is the name of the metric used to report when a SBOM is being generated at runtime
	// Tags: -
	MetricSBOMResolverSBOMGenerations = newRuntimeMetric(".sbom_resolver.sbom_generations")
	// MetricSBOMResolverFailedSBOMGenerations is the name of the metric used to report when a SBOM generation failed
	// Tags: -
	MetricSBOMResolverFailedSBOMGenerations = newRuntimeMetric(".sbom_resolver.failed_sbom_generations")
	// MetricSBOMResolverSBOMCacheLen is the name of the metric used to report the count of SBOMs kept in cache
	// Tags: -
	MetricSBOMResolverSBOMCacheLen = newRuntimeMetric(".sbom_resolver.sbom_cache.len")
	// MetricSBOMResolverSBOMCacheHit is the name of the metric used to report the number of SBOMs that were generated from cache
	// Tags: -
	MetricSBOMResolverSBOMCacheHit = newRuntimeMetric(".sbom_resolver.sbom_cache.hit")
	// MetricSBOMResolverSBOMCacheMiss is the name of the metric used to report the number of SBOMs that weren't in cache
	// Tags: -
	MetricSBOMResolverSBOMCacheMiss = newRuntimeMetric(".sbom_resolver.sbom_cache.miss")

	// Security Profile metrics

	// MetricSecurityProfileActiveProfiles is the name of the metric used to report the count of active Security Profiles
	// Tags: -
	MetricSecurityProfileActiveProfiles = newRuntimeMetric(".security_profile.active_profiles")
	// MetricSecurityProfileCacheLen is the name of the metric used to report the size of the Security Profile cache
	// Tags: -
	MetricSecurityProfileCacheLen = newRuntimeMetric(".security_profile.cache.len")
	// MetricSecurityProfileCacheHit is the name of the metric used to report the count of Security Profile cache hits
	// Tags: -
	MetricSecurityProfileCacheHit = newRuntimeMetric(".security_profile.cache.hit")
	// MetricSecurityProfileCacheMiss is the name of the metric used to report the count of Security Profile cache misses
	// Tags: -
	MetricSecurityProfileCacheMiss = newRuntimeMetric(".security_profile.cache.miss")

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

	// Policies

	// MetricRuleSetLoaded is the name of the metric used to report that a new ruleset was loaded
	// Tags: -
	MetricRuleSetLoaded = newRuntimeMetric(".ruleset_loaded")
	// MetricPolicy is the name of the metric used to report policy versions
	// Tags: -
	MetricPolicy = newRuntimeMetric(".policy")
	// MetricRuleStatus is the name of the metric used to report the rule status
	// Tags: -
	MetricRulesStatus = newRuntimeMetric(".rules_status")

	// Others

	// MetricSelfTest is the name of the metric used to report that a self test was performed
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

	// MetricSecurityAgentRuntimeContainersRunning is used to report the count of running containers when the security agent.
	// `Runtime` feature is enabled
	MetricSecurityAgentRuntimeContainersRunning = newAgentMetric(".runtime.containers_running")
	// MetricSecurityAgentFIMContainersRunning is used to report the count of running containers when the security agent
	// `FIM` feature is enabled
	MetricSecurityAgentFIMContainersRunning = newAgentMetric(".fim.containers_running")
	// MetricRuntimeCgroupsRunning is used to report the count of running cgroups.
	// Tags: -
	MetricRuntimeCgroupsRunning = newAgentMetric(".runtime.cgroups_running")

	// Event Monitoring metrics

	// MetricEventMonitoringRunning is reported when the runtime-security module is running with event monitoring enabled
	MetricEventMonitoringRunning = newAgentMetric(".event_monitoring.running")

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
	return append(tags, fmt.Sprintf("%s:%s", dogstatsdServer.CardinalityTagPrefix, cardinality))
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

	// ProcessSourceEventTags is assigned to metrics for process cache entries created from events
	ProcessSourceEventTags = []string{"type:event"}
	// ProcessSourceKernelMapsTags is assigned to metrics for process cache entries populated from kernel maps
	ProcessSourceKernelMapsTags = []string{KernelMapsTag}
	// ProcessSourceProcTags is assigned to metrics for process cache entries populated from /proc data
	ProcessSourceProcTags = []string{ProcFSTag}
)

func newRuntimeMetric(name string) string {
	return MetricRuntimePrefix + name
}

func newAgentMetric(name string) string {
	return MetricAgentPrefix + name
}
