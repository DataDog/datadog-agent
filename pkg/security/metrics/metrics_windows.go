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

	//MetricWindowsFileCreate is the metric for counting file create notifications
	//Tags: -
	MetricWindowsFileCreate = newRuntimeMetric(".windows.file.create")
	//MetricWindowsFileCreateNew is the metric for counting file create notifications for new files
	//Tags: -
	MetricWindowsFileCreateNew = newRuntimeMetric(".windows.file.create_new")
	//MetricWindowsFileCleanup is the metric for counting file cleanup notifications
	//Tags: -
	MetricWindowsFileCleanup = newRuntimeMetric(".windows.file.cleanup")
	//MetricWindowsFileResolverOverwrite is the metric for counting file overwrite notifications
	//Tags: -
	MetricWindowsFileResolverOverwrite = newRuntimeMetric(".windows.file_resolver.overwrite")
	//MetricWindowsFileResolverNew is the metric for counting file create notifications for new files
	//Tags: -
	MetricWindowsFileResolverNew = newRuntimeMetric(".windows.file_resolver.new")

	//MetricWindowsFileClose is the metric for counting file close notifications
	//Tags: -
	MetricWindowsFileClose = newRuntimeMetric(".windows.file.close")
	//MetricWindowsFileFlush is the metric for counting file flush notifications
	//Tags: -
	MetricWindowsFileFlush = newRuntimeMetric(".windows.file.flush")
	//MetricWindowsFileWrite is the metric for counting file write notifications
	//Tags: -
	MetricWindowsFileWrite = newRuntimeMetric(".windows.file.write")
	//MetricWindowsFileWriteProcessed is the metric for counting file write notifications
	//Tags: -
	MetricWindowsFileWriteProcessed = newRuntimeMetric(".windows.file.write_processed")

	//MetricWindowsFileSetInformation is the metric for counting file set information notifications
	//Tags: -
	MetricWindowsFileSetInformation = newRuntimeMetric(".windows.file.set_information")
	//MetricWindowsFileSetDelete is the metric for counting file set delete notifications
	//Tags: -
	MetricWindowsFileSetDelete = newRuntimeMetric(".windows.file.set_delete")
	//MetricWindowsFileSetRename is the metric for counting file set rename notifications
	//Tags: -
	MetricWindowsFileSetRename = newRuntimeMetric(".windows.file.set_rename")
	//MetricWindowsFileIDRename is the metric for counting file id rename notifications
	//Tags: -
	MetricWindowsFileIDRename = newRuntimeMetric(".windows.file.id_rename")
	//MetricWindowsFileIDQueryInformation is the metric for counting file id query information notifications
	//Tags: -
	MetricWindowsFileIDQueryInformation = newRuntimeMetric(".windows.file.id_query_information")
	//MetricWindowsFileIDFSCTL is the metric for counting file id fsctl notifications
	//Tags: -
	MetricWindowsFileIDFSCTL = newRuntimeMetric(".windows.file.id_fsctl")
	//MetricWindowsFileIDRename29 is the metric for counting file id rename notifications
	//Tags: -
	MetricWindowsFileIDRename29 = newRuntimeMetric(".windows.file.id_rename29")

	//MetricWindowsFileCreateSkippedDiscardedPaths is the metric for counting file create notifications for skipped discarded paths
	//Tags: -
	MetricWindowsFileCreateSkippedDiscardedPaths = newRuntimeMetric(".windows.file.create_skipped_discarded_paths")

	//MetricWindowsFileCreateSkippedDiscardedBasenames is the metric for counting file create notifications for skipped discarded basenames
	//Tags: -
	MetricWindowsFileCreateSkippedDiscardedBasenames = newRuntimeMetric(".windows.file.create_skipped_discarded_basenames")

	//MetricWindowsRegCreateKey is the metric for counting registry key create notifications
	//Tags: -
	MetricWindowsRegCreateKey = newRuntimeMetric(".windows.registry.create_key")
	//MetricWindowsRegOpenKey is the metric for counting registry key open notifications
	//Tags: -
	MetricWindowsRegOpenKey = newRuntimeMetric(".windows.registry.open_key")
	//MetricWindowsRegDeleteKey is the metric for counting registry key delete notifications
	//Tags: -
	MetricWindowsRegDeleteKey = newRuntimeMetric(".windows.registry.delete_key")
	//MetricWindowsRegFlushKey is the metric for counting registry key flush notifications
	//Tags: -
	MetricWindowsRegFlushKey = newRuntimeMetric(".windows.registry.flush_key")
	//MetricWindowsRegCloseKey is the metric for counting registry key close notifications
	//Tags: -
	MetricWindowsRegCloseKey = newRuntimeMetric(".windows.registry.close_key")
	//MetricWindowsRegSetValue is the metric for counting registry value set notifications
	//Tags: -
	MetricWindowsRegSetValue = newRuntimeMetric(".windows.registry.set_value")
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
)
