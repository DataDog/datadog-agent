// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics holds metrics related files
package metrics

import (
	"strings"
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

	// Rate limiter metrics

	// MetricRateLimiterDrop is the name of the metric used to count the amount of events dropped by the rate limiter
	// Tags: rule_id
	MetricRateLimiterDrop = newRuntimeMetric(".rules.rate_limiter.drop")
	// MetricRateLimiterAllow is the name of the metric used to count the amount of events allowed by the rate limiter
	// Tags: rule_id
	MetricRateLimiterAllow = newRuntimeMetric(".rules.rate_limiter.allow")

	// MetricRulesNoMatch is the number of events that reached userspace but didn't match any rule
	// Tags: event_type, category
	MetricRulesNoMatch = newRuntimeMetric(".rules.no_match")

	// Rule action metrics

	// MetricRuleActionPerformed is the name of the metric used to count actions performed after a rule was matched
	// Tags: rule_id, action_name
	MetricRuleActionPerformed = newRuntimeMetric(".rules.action_performed")

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
	// MetricDentryCacheSize is the size of the cache
	MetricDentryCacheSize = newRuntimeMetric(".dentry_resolver.cache_size")

	// DNS Resolver metrics

	// MetricDNSResolverIPResolverCache is the counter for the IP resolver (A and AAAA records)
	// Tags: hit, miss, insertion, eviction
	MetricDNSResolverIPResolverCache = newRuntimeMetric(".dns_resolver.ip_resolver_cache")
	// MetricDNSResolverCnameResolverCache is the counter for the CNAME resolver
	// Tags: hit, miss, insertion, eviction
	MetricDNSResolverCnameResolverCache = newRuntimeMetric(".dns_resolver.cname_resolver_cache")

	// MetricRepeatedDNSResponsesFilteredOnKernel DNS responses that were filtered on the kernel
	MetricRepeatedDNSResponsesFilteredOnKernel = newRuntimeMetric(".dns_response_collector.repeated_dns_responses_filtered_on_kernel")
	// MetricDNSSameIDDifferentSize DNS responses that had the same ID but a different size
	MetricDNSSameIDDifferentSize = newRuntimeMetric(".dns_response_collector.dns_same_id_different_size")
	// MetricDiscardedDNSPackets DNS responses that were discarded because of not matching a rule
	MetricDiscardedDNSPackets = newRuntimeMetric(".dns_response_collector.dns_discarded_packets")

	// filtering metrics

	// MetricDiscarderAdded is the number of discarder added
	// Tags: discarder_type, event_type
	MetricDiscarderAdded = newRuntimeMetric(".discarders.discarder_added")
	// MetricEventDiscarded is the number of event discarded
	// Tags: discarder_type, event_type
	MetricEventDiscarded = newRuntimeMetric(".discarders.event_discarded")
	// MetricApproverAdded is the number of approvers added
	// Tags: approver_type, event_type
	MetricApproverAdded = newRuntimeMetric(".approvers.approver_added")
	// MetricEventApproved is the number of events approved
	// Tags: approver_type, event_type
	MetricEventApproved = newRuntimeMetric(".approvers.event_approved")
	// MetricEventRejected is the number of events rejected
	// Tags: event_type
	MetricEventRejected = newRuntimeMetric(".approvers.event_rejected")

	// syscalls metrics

	// MetricSyscallsInFlight is the number of inflight events
	// Tags: event_type
	MetricSyscallsInFlight = newRuntimeMetric(".syscalls_map.event_inflight")

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

	// MetricPerfBufferInvalidEventsCount is the name of the metric used to count the number of invalid events retrieved from the event stream
	// Tags: map, cause
	MetricPerfBufferInvalidEventsCount = newRuntimeMetric(".perf_buffer.invalid_events.count")
	// MetricPerfBufferInvalidEventsBytes is the name of the metric used to count the number of bytes of invalid events retrieved from the event stream
	// Tags: map, cause
	MetricPerfBufferInvalidEventsBytes = newRuntimeMetric(".perf_buffer.invalid_events.bytes")

	// Process Resolver metrics

	// MetricProcessResolverCacheSize is the name of the metric used to report the size of the user space
	// process cache
	// Tags: -
	MetricProcessResolverCacheSize = newRuntimeMetric(".process_resolver.cache_size")
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
	// MetricProcessInodeError is the name of the metric used to report a broken lineage with a inode mismatch
	// Tags: -
	MetricProcessInodeError = newRuntimeMetric(".process_resolver.inode_error")
	// MetricProcessResolverReparentSuccess counts successful process reparenting
	// Tags: callpath:set_process_context, callpath:do_exit, callpath:dequeue_exited, callpath:event_serialization
	MetricProcessResolverReparentSuccess = newRuntimeMetric(".process_resolver.reparent.success")
	// MetricProcessResolverReparentFailed counts failed reparenting attempts (e.g. procfs not updated yet)
	// Tags: callpath:set_process_context, callpath:do_exit, callpath:dequeue_exited, callpath:event_serialization
	MetricProcessResolverReparentFailed = newRuntimeMetric(".process_resolver.reparent.failed")

	// Mount resolver metrics

	// MetricMountResolverCacheSize is the name of the metric used to report the size of the user space
	// mount cache
	// Tags: -
	MetricMountResolverCacheSize = newRuntimeMetric(".mount_resolver.cache_size")
	// MetricMountResolverHits is the counter of successful mount resolution
	// Tags: cache, procfs
	MetricMountResolverHits = newRuntimeMetric(".mount_resolver.hits")
	// MetricMountResolverMiss is the counter of unsuccessful mount resolution
	// Tags: cache, procfs
	MetricMountResolverMiss = newRuntimeMetric(".mount_resolver.miss")
	// MetricMountResolverMiss is the counter of unsuccessful procfs mount resolution
	// Tags: cache, procfs
	MetricMountResolverProcfsMiss = newRuntimeMetric(".mount_resolver.procfs_miss")
	// MetricMountResolverProcfsHits is the counter of successful procfs mount resolution
	// Tags: cache, procfs
	MetricMountResolverProcfsHits = newRuntimeMetric(".mount_resolver.procfs_hits")

	// Activity dump metrics

	// MetricActivityDumpEventProcessed is the name of the metric used to count the number of events processed while
	// creating an activity dump.
	// Tags: event_type, tree_type
	MetricActivityDumpEventProcessed = newRuntimeMetric(".activity_dump.event.processed")
	// MetricActivityDumpEventAdded is the name of the metric used to count the number of events that were added to an
	// activity dump.
	// Tags: event_type, tree_type
	MetricActivityDumpEventAdded = newRuntimeMetric(".activity_dump.event.added")
	// MetricActivityDumpEventDropped is the name of the metric used to count the number of events that were dropped from an
	// activity dump.
	// Tags: event_type, reason, tree_type
	MetricActivityDumpEventDropped = newRuntimeMetric(".activity_dump.event.dropped")
	// MetricActivityDumpSizeInBytes is the name of the metric used to report the size of the generated activity dumps in
	// bytes
	// Tags: format, storage_type, compression
	MetricActivityDumpSizeInBytes = newRuntimeMetric(".activity_dump.size_in_bytes")
	// MetricActivityDumpPersistedDumps is the name of the metric used to reported the number of dumps that were persisted
	// Tags: format, storage_type, compression
	MetricActivityDumpPersistedDumps = newRuntimeMetric(".activity_dump.persisted_dumps")
	// MetricActivityDumpActiveDumps is the name of the metric used to report the number of active dumps
	// Tags: -
	MetricActivityDumpActiveDumps = newRuntimeMetric(".activity_dump.active_dumps")
	// MetricActivityDumpActiveDumpSizeInMemory is the size of an activity dump in memory
	// Tags: dump_index
	MetricActivityDumpActiveDumpSizeInMemory = newRuntimeMetric(".activity_dump.size_in_memory")
	// MetricActivityDumpEntityTooLarge is the name of the metric used to report the number of active dumps that couldn't
	// be sent because they are too big
	// Tags: format, compression
	MetricActivityDumpEntityTooLarge = newAgentMetric(".activity_dump.entity_too_large")
	// MetricActivityDumpEmptyDropped is the name of the metric used to report the number of activity dumps dropped because they were empty
	// Tags: -
	MetricActivityDumpEmptyDropped = newRuntimeMetric(".activity_dump.empty_dump_dropped")
	// MetricActivityDumpDropMaxDumpReached is the name of the metric used to report that an activity dump was dropped because the maximum amount of dumps for a workload was reached
	// Tags: -
	MetricActivityDumpDropMaxDumpReached = newRuntimeMetric(".activity_dump.drop_max_dump_reached")
	// MetricActivityDumpNotYetProfiledWorkload is the name of the metric used to report the count of workload not yet profiled
	// Tags: -
	MetricActivityDumpNotYetProfiledWorkload = newAgentMetric(".activity_dump.not_yet_profiled_workload")
	// MetricActivityDumpWorkloadDenyListHits is the name of the metric used to report the count of dumps that were dismissed because their workload is in the deny list
	// Tags: -
	MetricActivityDumpWorkloadDenyListHits = newRuntimeMetric(".activity_dump.workload_deny_list_hits")
	// MetricActivityDumpLocalStorageCount is the name of the metric used to count the number of dumps stored locally
	// Tags: -
	MetricActivityDumpLocalStorageCount = newAgentMetric(".activity_dump.local_storage.count")
	// MetricActivityDumpLocalStorageDeleted is the name of the metric used to track the deletion of workload entries in
	// the local storage.
	// Tags: -
	MetricActivityDumpLocalStorageDeleted = newAgentMetric(".activity_dump.local_storage.deleted")

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

	// CGroup resolver metrics

	// MetricCGroupResolverActiveCGroups is the name of the metric used to report the count of cgroups kept in memory
	// Tags: -
	MetricCGroupResolverActiveCGroups = newRuntimeMetric(".cgroup_resolver.active_cgroups")
	// MetricCGroupResolverActiveContainerWorkloads is the name of the metric used to report the count of active cgroups corresponding to a container kept in memory
	// Tags: -
	MetricCGroupResolverActiveContainerWorkloads = newRuntimeMetric(".cgroup_resolver.active_containers")
	// MetricCGroupResolverActiveHostWorkloads is the name of the metric used to report the count of active cgroups not corresponding to a container kept in memory
	// Tags: -
	MetricCGroupResolverActiveHostWorkloads = newRuntimeMetric(".cgroup_resolver.active_non_containers")
	// MetricCGroupResolverAddedCgroups is the name of the metric used to report the number of added cgroups
	// Tags: -
	MetricCGroupResolverAddedCgroups = newRuntimeMetric(".cgroup_resolver.added_cgroups")
	// MetricCGroupResolverDeletedCgroups is the name of the metric used to report the number of deleted cgroups
	// Tags: -
	MetricCGroupResolverDeletedCgroups = newRuntimeMetric(".cgroup_resolver.deleted_cgroups")
	// MetricCGroupResolverFallbackSucceed is the name of the metric used to report the number of succeed fallbacks
	// Tags: -
	MetricCGroupResolverFallbackSucceed = newRuntimeMetric(".cgroup_resolver.fallback_succeed")
	// MetricCGroupResolverFallbackFailed is the name of the metric used to report the number of failed fallbacks
	// Tags: -
	MetricCGroupResolverFallbackFailed = newRuntimeMetric(".cgroup_resolver.fallback_failed")

	// Security Profile metrics

	// MetricSecurityProfileProfiles is the name of the metric used to report the count of Security Profiles per category
	// Tags: in_kernel (true or false), anomaly_detection (true or false), workload_hardening (true or false)
	MetricSecurityProfileProfiles = newRuntimeMetric(".security_profile.profiles")
	// MetricSecurityProfileCacheLen is the name of the metric used to report the size of the Security Profile cache
	// Tags: -
	MetricSecurityProfileCacheLen = newRuntimeMetric(".security_profile.cache.len")
	// MetricSecurityProfileCacheHit is the name of the metric used to report the count of Security Profile cache hits
	// Tags: -
	MetricSecurityProfileCacheHit = newRuntimeMetric(".security_profile.cache.hit")
	// MetricSecurityProfileCacheMiss is the name of the metric used to report the count of Security Profile cache misses
	// Tags: -
	MetricSecurityProfileCacheMiss = newRuntimeMetric(".security_profile.cache.miss")
	// MetricSecurityProfileEventFiltering is the name of the metric used to report the count of Security Profile event filtered
	// Tags: event_type, profile_state ('no_profile', 'unstable', 'unstable_event_type', 'stable', 'auto_learning', 'workload_warmup'), in_profile ('true', 'false' or none)
	MetricSecurityProfileEventFiltering = newRuntimeMetric(".security_profile.evaluation.hit")
	// MetricSecurityProfileDirectoryProviderCount is the name of the metric used to track the count of profiles in the cache
	// of the Profile directory provider
	// Tags: -
	MetricSecurityProfileDirectoryProviderCount = newAgentMetric(".activity_dump.directory_provider.count")
	// MetricSecurityProfileEvictedVersions is the name of the metric used to track the evicted profile versions
	// Tags: image_name, image_tag
	MetricSecurityProfileEvictedVersions = newAgentMetric(".security_profile.evicted_versions")
	// MetricSecurityProfileVersions is the name of the metric used to track the number of versions a profile can have
	// Tags: security_profile_image_name
	MetricSecurityProfileVersions = newAgentMetric(".security_profile.versions")

	// Hash resolver metrics

	// MetricHashResolverHashCount is the name of the metric used to report the count of hashes generated by the hash
	// resolver
	// Tags: event_type, hash
	MetricHashResolverHashCount = newITRuntimeMetric("hash_resolver", "count")
	// MetricHashResolverHashMiss is the name of the metric used to report the amount of times we failed to compute a hash
	// Tags: event_type, reason
	MetricHashResolverHashMiss = newITRuntimeMetric("hash_resolver", "miss")
	// MetricHashResolverHashCacheHit is the name of the metric used to report the amount of times the cache was used
	// Tags: event_type
	MetricHashResolverHashCacheHit = newITRuntimeMetric("hash_resolver", "cache_hit")
	// MetricHashResolverHashCacheLen is the name of the metric used to report the count of hashes in cache
	// Tags: -
	MetricHashResolverHashCacheLen = newITRuntimeMetric("hash_resolver", "cache_len")

	// File resolver metrics

	// MetricFileResolverCacheHit is the name of the metric used to report file resolver cache hits
	// Tags: -
	MetricFileResolverCacheHit = newRuntimeMetric(".file_resolver.cache_hit")
	// MetricFileResolverCacheMiss is the name of the metric used to report file resolver cache misses
	// Tags: -
	MetricFileResolverCacheMiss = newRuntimeMetric(".file_resolver.cache_miss")

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
	// MetricRulesStatus is the name of the metric used to report the rule status
	// Tags: -
	MetricRulesStatus = newRuntimeMetric(".rules_status")
	// MetricSECLTotalVariables tracks the total number of SECL variables
	// Tags: type ('bool', 'integer', 'string', 'ip', 'strings', 'integers', 'ips'), scope ('global', 'process', 'cgroup', 'container')
	MetricSECLTotalVariables = newITRuntimeMetric("rule_engine", "total_variables")

	// Enforcement metrics

	// MetricEnforcementKillQueued is the name of the metric used to report the number of kill action queued
	// Tags: rule_id
	MetricEnforcementKillQueued = newRuntimeMetric(".enforcement.kill_queued")
	// MetricEnforcementKillQueuedDiscarded is the name of the metric used to report the number of kill action queued which has been discarded due to a rule disarm
	// Tags: rule_id
	MetricEnforcementKillQueuedDiscarded = newRuntimeMetric(".enforcement.kill_queued_discarded")
	// MetricEnforcementProcessKilled is the name of the metric used to report the number of processes killed
	// Tags: rule_id, queued:true/false
	MetricEnforcementProcessKilled = newRuntimeMetric(".enforcement.process_killed")
	// MetricEnforcementRuleDisarmed is the name of the metric used to report that a rule was disarmed
	// Tags: rule_id, disarmer_type ('executable', 'container')
	MetricEnforcementRuleDisarmed = newRuntimeMetric(".enforcement.rule_disarmed")
	// MetricEnforcementRuleDismantled is the name of the metric used to report that a rule was dismantled
	// Tags: rule_id, disarmer_type ('executable', 'container')
	MetricEnforcementRuleDismantled = newRuntimeMetric(".enforcement.rule_dismantled")
	// MetricEnforcementRuleRearmed is the name of the metric used to report that a rule was rearmed
	// Tags: rule_id
	MetricEnforcementRuleRearmed = newRuntimeMetric(".enforcement.rule_rearmed")

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
	// MetricSecurityAgentFargateFIMRunning is reported when the security agent `FIM` feature is enabled on Fargate
	MetricSecurityAgentFargateFIMRunning = newAgentMetric(".fargate_fim.running")
	// MetricSecurityAgentFargateRuntimeRunning is reported when the security agent `Runtime` feature is enabled on Fargate
	MetricSecurityAgentFargateRuntimeRunning = newAgentMetric(".fargate_runtime.running")
	// MetricSecurityAgentRuntimeContainersRunning is used to report the count of running containers when the security agent.
	// `Runtime` feature is enabled
	MetricSecurityAgentRuntimeContainersRunning = newAgentMetric(".runtime.containers_running")
	// MetricSecurityAgentFargateRuntimeContainersRunning is used to report the count of running containers when the security agent.
	// `Runtime` feature is enabled on Fargate
	MetricSecurityAgentFargateRuntimeContainersRunning = newAgentMetric(".fargate_runtime.containers_running")
	// MetricSecurityAgentFIMContainersRunning is used to report the count of running containers when the security agent
	// `FIM` feature is enabled
	MetricSecurityAgentFIMContainersRunning = newAgentMetric(".fim.containers_running")
	// MetricSecurityAgentFargateFIMContainersRunning is used to report the count of running containers when the security agent
	// `FIM` feature is enabled on Fargate
	MetricSecurityAgentFargateFIMContainersRunning = newAgentMetric(".fargate_fim.containers_running")
	// MetricRuntimeCgroupsRunning is used to report the count of running cgroups.
	// Tags: -
	MetricRuntimeCgroupsRunning = newAgentMetric(".runtime.cgroups_running")

	// Event Monitoring metrics

	// MetricEventMonitoringRunning is reported when the runtime-security module is running with event monitoring enabled
	MetricEventMonitoringRunning = newAgentMetric(".event_monitoring.running")
	// MetricEventMonitoringEventsDropped is the name of the metric used to count the number of bytes of event dropped
	// Tags: consumer_id
	MetricEventMonitoringEventsDropped = newRuntimeMetric(".event_monitoring.events.dropped")

	//BPFFilter metrics

	//MetricBPFFilterTruncated is the name of the metric used to report truncated BPF filter
	// Tags: -
	MetricBPFFilterTruncated = newRuntimeMetric(".bpf_filter.truncated")

	// PrCtl metrics

	// MetricNameTruncated is the name of the metric used to report truncated name used in prctl
	// Tags: -
	MetricNameTruncated = newRuntimeMetric(".prctl.name_truncated")

	// Security Profile V2 metrics

	// Event Processing metrics

	// MetricSecurityProfileV2EventsReceived is the name of the metric used to report events received by ProcessEvent (after filters)
	// Tags: source (runtime or replay)
	MetricSecurityProfileV2EventsReceived = newRuntimeMetric(".security_profile_v2.events.received")

	// MetricSecurityProfileV2EventsImmediate is the name of the metric used to report events processed immediately (tags already resolved)
	// Tags: source (runtime or replay)
	MetricSecurityProfileV2EventsImmediate = newRuntimeMetric(".security_profile_v2.events.immediate")

	// Tag Resolution metrics

	// MetricSecurityProfileV2TagResolutionEventsQueued is the name of the metric used to report the total events queued waiting for tag resolution
	// Tags: -
	MetricSecurityProfileV2TagResolutionEventsQueued = newRuntimeMetric(".security_profile_v2.tag_resolution.events_queued")

	// MetricSecurityProfileV2TagResolutionCgroupsPending is the name of the metric used to report the number of cgroups waiting for tag resolution
	// Tags: -
	MetricSecurityProfileV2TagResolutionCgroupsPending = newRuntimeMetric(".security_profile_v2.tag_resolution.cgroups_pending")

	// MetricSecurityProfileV2TagResolutionCgroupsResolved is the name of the metric used to report current cgroups with resolved tags (actively profiled)
	// Tags: - (Gauge)
	MetricSecurityProfileV2TagResolutionCgroupsResolved = newRuntimeMetric(".security_profile_v2.tag_resolution.cgroups_resolved")

	// MetricSecurityProfileV2TagResolutionEventsDropped is the name of the metric used to report events dropped due to 10s stale timeout
	// Tags: source (runtime or replay)
	MetricSecurityProfileV2TagResolutionEventsDropped = newRuntimeMetric(".security_profile_v2.tag_resolution.events_dropped")

	// MetricSecurityProfileV2TagResolutionCgroupsExpired is the name of the metric used to report cgroups cleaned up after 60s without ever resolving tags
	// Tags: -
	MetricSecurityProfileV2TagResolutionCgroupsExpired = newRuntimeMetric(".security_profile_v2.tag_resolution.cgroups_expired")

	// MetricSecurityProfileV2TagResolutionLatency is the name of the metric used to report the time between first event and successful tag resolution
	// Tags: -
	MetricSecurityProfileV2TagResolutionLatency = newRuntimeMetric(".security_profile_v2.tag_resolution.latency")

	// Event Processing metrics

	// MetricSecurityProfileV2EventsDroppedMaxSize is the name of the metric used to report events dropped because profile reached max size
	// Tags: -
	MetricSecurityProfileV2EventsDroppedMaxSize = newRuntimeMetric(".security_profile_v2.events.dropped_max_size")

	// Persistence metrics

	// MetricSecurityProfileV2SizeInBytes is the name of the metric used to report the size of generated security profiles in bytes
	// Tags: format, storage_type, compression
	MetricSecurityProfileV2SizeInBytes = newRuntimeMetric(".security_profile_v2.size_in_bytes")

	// MetricSecurityProfileV2PersistedProfiles is the name of the metric used to report the number of profiles that were persisted
	// Tags: format, storage_type, compression
	MetricSecurityProfileV2PersistedProfiles = newRuntimeMetric(".security_profile_v2.persisted_profiles")

	// Eviction metrics

	// MetricSecurityProfileV2EvictionRuns is the name of the metric used to report the number of eviction cycles run
	// Tags: -
	MetricSecurityProfileV2EvictionRuns = newRuntimeMetric(".security_profile_v2.eviction.runs")

	// MetricSecurityProfileV2EvictionNodesEvictedPerProfile is the name of the metric used to report nodes evicted from a specific profile
	// Tags: -
	MetricSecurityProfileV2EvictionNodesEvictedPerProfile = newRuntimeMetric(".security_profile_v2.eviction.nodes_evicted_per_profile")

	// Profile cleanup metrics

	// MetricSecurityProfileV2CleanupProfilesRemoved is the name of the metric used to report profiles removed after cleanup delay
	// Tags: -
	MetricSecurityProfileV2CleanupProfilesRemoved = newRuntimeMetric(".security_profile_v2.cleanup.profiles_removed")
)

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

	// ReparentCallpathSetProcessContext tags a reparent from the setProcessContext path (lazy repair)
	ReparentCallpathSetProcessContext = "callpath:set_process_context"
	// ReparentCallpathDoExit tags a reparent from the ApplyExitEntry path (do_exit)
	ReparentCallpathDoExit = "callpath:do_exit"
	// ReparentCallpathDequeueExited tags a reparent from the DequeueExited path (cleanup goroutine)
	ReparentCallpathDequeueExited = "callpath:dequeue_exited"
	// ReparentCallpathEventSerialization tags a reparent from the event serialization path
	ReparentCallpathEventSerialization = "callpath:event_serialization"
	// AllReparentCallpathTags is the list of all reparent callpath tags
	AllReparentCallpathTags = []string{ReparentCallpathSetProcessContext, ReparentCallpathDoExit, ReparentCallpathDequeueExited, ReparentCallpathEventSerialization}
)

func newRuntimeMetric(name string) string {
	return MetricRuntimePrefix + name
}

func newAgentMetric(name string) string {
	return MetricAgentPrefix + name
}

// ITMetric is a struct that represents a metric for internal telemetry
type ITMetric struct {
	// Subsystem is the subsystem of the metric, used to group related metrics
	Subsystem string
	// Name is the name of the metric, used to identify it
	Name string
}

func newITRuntimeMetric(subsystem string, name string) ITMetric {
	name = strings.ReplaceAll(name, ".", "__")
	return ITMetric{
		Subsystem: "runtime_security__" + subsystem,
		Name:      name,
	}
}
