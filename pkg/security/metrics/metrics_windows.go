// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics holds metrics related files
package metrics

var (
	//MetricWindowsProcessStart is the metric for counting process start notifications
	//Tags: -
	MetricWindowsProcessStart = newRuntimeMetric(".windows.process.start")
	//MetricWindowsProcessStop is the metric for counting process stop notifications
	//Tags: -
	MetricWindowsProcessStop = newRuntimeMetric(".windows.process.stop")

	//MetricWindowsFileResolverOverwrite is the metric for counting file create notifications for cache overwrites
	//Tags: -
	MetricWindowsFileResolverOverwrite = newRuntimeMetric(".windows.file_resolver.overwrite")

	//MetricWindowsFileResolverNew is the metric for counting file create notifications for new files
	//Tags: -
	MetricWindowsFileResolverNew = newRuntimeMetric(".windows.file_resolver.new")

	//MetricWindowsFileCreateSkippedDiscardedPaths is the metric for counting file create notifications for skipped discarded paths
	//Tags: -
	MetricWindowsFileCreateSkippedDiscardedPaths = newRuntimeMetric(".windows.file.create_skipped_discarded_paths")

	//MetricWindowsFileCreateSkippedDiscardedBasenames is the metric for counting file create notifications for skipped discarded basenames
	//Tags: -
	MetricWindowsFileCreateSkippedDiscardedBasenames = newRuntimeMetric(".windows.file.create_skipped_discarded_basenames")

	//MetricWindowsSizeOfFilePathResolver is the metric for counting the size of the file cache
	//Tags: -
	MetricWindowsSizeOfFilePathResolver = newRuntimeMetric(".windows.file_resolver.size")
	//MetricWindowsSizeOfRegistryPathResolver is the metric for counting the size of the registry cache
	//Tags: -
	MetricWindowsSizeOfRegistryPathResolver = newRuntimeMetric(".windows.registry_resolver.size")
	//MetricWindowsETWChannelBlockedCount is the metric for counting the number of blocked ETW channels
	//Tags: -
	MetricWindowsETWChannelBlockedCount = newRuntimeMetric(".windows.etw_channel_blocked_count")
	//MetricWindowsETWNumberOfBuffers is the metric for counting the number of ETW buffers
	//Tags: -
	MetricWindowsETWNumberOfBuffers = newRuntimeMetric(".windows.etw_number_of_buffers")
	//MetricWindowsETWFreeBuffers is the metric for counting the number of free ETW buffers
	//Tags: -
	MetricWindowsETWFreeBuffers = newRuntimeMetric(".windows.etw_free_buffers")
	//MetricWindowsETWEventsLost is the metric for counting the number of ETW events lost
	//Tags: -
	MetricWindowsETWEventsLost = newRuntimeMetric(".windows.etw_events_lost")
	//MetricWindowsETWBuffersWritten is the metric for counting the number of ETW buffers written
	//Tags: -
	MetricWindowsETWBuffersWritten = newRuntimeMetric(".windows.etw_buffers_written")
	//MetricWindowsETWLogBuffersLost is the metric for counting the number of ETW log buffers lost
	//Tags: -
	MetricWindowsETWLogBuffersLost = newRuntimeMetric(".windows.etw_log_buffers_lost")
	//MetricWindowsETWRealTimeBuffersLost is the metric for counting the number of ETW real-time buffers lost
	//Tags: -
	MetricWindowsETWRealTimeBuffersLost = newRuntimeMetric(".windows.etw_real_time_buffers_lost")
	//MetricWindowsETWTotalNotifications is the metric for counting the total number of ETW notifications
	//Tags: -
	MetricWindowsETWTotalNotifications = newRuntimeMetric(".windows.etw_total_notifications")
	//MetricWindowsFilePathEvictions is the metric for counting the number of file path evictions
	//Tags: -
	MetricWindowsFilePathEvictions = newRuntimeMetric(".windows.file_path_evictions")
	//MetricWindowsRegPathEvictions is the metric for counting the number of registry path evictions
	//Tags: -
	MetricWindowsRegPathEvictions = newRuntimeMetric(".windows.registry_path_evictions")
	//MetricWindowsFileNotifications is the metric for counting the number of file notifications received
	//Tags: -
	MetricWindowsFileNotifications = newRuntimeMetric(".windows.file_notifications")
	//MetricWindowsFileNotificationsProcessed is the metric for counting the number of file notifications processed
	//Tags: -
	MetricWindowsFileNotificationsProcessed = newRuntimeMetric(".windows.file_notifications_processed")
	//MetricWindowsRegistryNotifications is the metric for counting the number of registry notifications received
	//Tags: -
	MetricWindowsRegistryNotifications = newRuntimeMetric(".windows.registry_notifications")
	//MetricWindowsRegistryNotificationsProcessed is the metric for counting the number of registry notifications processed
	//Tags: -
	MetricWindowsRegistryNotificationsProcessed = newRuntimeMetric(".windows.registry_notifications_processed")
	//MetricWindowsApproverRejects is the metric for counting the number of approver rejects
	//Tags: -
	MetricWindowsApproverRejects = newRuntimeMetric(".windows.approver_rejects")

	//MetricWindowsEventCacheOverflow is the metric for counting the number of event cache overflows
	//Tags: -
	MetricWindowsEventCacheOverflow = newRuntimeMetric(".windows.event_cache_overflow")
	//MetricWindowEventCacheUnderflow is the metric for counting the number of event cache underflows
	//Tags: -
	MetricWindowsEventCacheUnderflow = newRuntimeMetric(".windows.event_cache_underflow")
)
