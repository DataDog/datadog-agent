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
	//MetricWindowsFileResolverOverwrite is the metric for counting file overwrite notifications
	//Tags: -
	MetricWindowsFileResolverOverwrite = newRuntimeMetric(".windows.file_resolver.overwrite")
	//MetricWindowsFileResolverNew is the metric for counting file create notifications for new files
	//Tags: -
	MetricWindowsFileResolverNew = newRuntimeMetric(".windows.file_resolver.new")

	MetricWindowsFileCleanup = newRuntimeMetric(".windows.file.cleanup")
	//MetricWindowsFileClose is the metric for counting file close notifications
	//Tags: -
	MetricWindowsFileClose = newRuntimeMetric(".windows.file.close")
	//MetricWindowsFileFlush is the metric for counting file flush notifications
	//Tags: -
	MetricWindowsFileFlush = newRuntimeMetric(".windows.file.flush")
	//MetricWindowsRegCreateKey is the metric for counting registry key create notifications
	//Tags: -
	//MetricWindowsFileSetInformation is the metric for counting file set information notifications
	//Tags: -
	MetricWindowsFileSetInformation = newRuntimeMetric(".windows.file.set_information")
	//MetricWindowsFileSetDelete is the metric for counting file set delete notifications
	//Tags: -
	MetricWindowsFileSetDelete = newRuntimeMetric(".windows.file.set_delete")
	//MetricWindowsFileSetRename is the metric for counting file set rename notifications
	//Tags: -
	MetricWindowsFileSetRename = newRuntimeMetric(".windows.file.set_rename")
	//MetricWindowsFileIdRename is the metric for counting file id rename notifications
	//Tags: -
	MetricWindowsFileIdRename = newRuntimeMetric(".windows.file.id_rename")
	//MetricWindowsFileIdQueryInformation is the metric for counting file id query information notifications
	//Tags: -
	MetricWindowsFileIdQueryInformation = newRuntimeMetric(".windows.file.id_query_information")
	//MetricWindowsFileIdFSCTL is the metric for counting file id fsctl notifications
	//Tags: -
	MetricWindowsFileIdFSCTL = newRuntimeMetric(".windows.file.id_fsctl")
	//MetricWindowsFileIdRename29 is the metric for counting file id rename notifications
	//Tags: -
	MetricWindowsFileIdRename29 = newRuntimeMetric(".windows.file.id_rename29")

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
	//MetricWindowsSizeOfFileCache is the metric for counting the size of the file cache
	//Tags: -
	MetricWindowsSizeOfFilePathResolver = newRuntimeMetric(".windows.file_resolver.size")
	//MetricWindowsSizeOfRegistryCache is the metric for counting the size of the registry cache
	//Tags: -
	MetricWindowsSizeOfRegistryPathResolver = newRuntimeMetric(".windows.registry_resolver.size")
)
