// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import "github.com/DataDog/datadog-agent/pkg/util/fxutil/startup"

// Each worker owns its counters. They are merged only after wg.Wait, avoiding
// recorder locks and per-file spans in the read/parse loop. Durations are summed
// elapsed times, not CPU times, and can exceed the worker pool's wall duration.
type configReadStats struct {
	directoriesRead int
	directoryErrors int
	filesAttempted  int
	filesRead       int
	readErrors      int
	emptyFiles      int
	configsParsed   int
	configErrors    int
	bytesRead       int64
	directoryReadNS int64
	fileReadNS      int64
	parseNS         int64
}

func (s *configReadStats) add(other configReadStats) {
	s.directoriesRead += other.directoriesRead
	s.directoryErrors += other.directoryErrors
	s.filesAttempted += other.filesAttempted
	s.filesRead += other.filesRead
	s.readErrors += other.readErrors
	s.emptyFiles += other.emptyFiles
	s.configsParsed += other.configsParsed
	s.configErrors += other.configErrors
	s.bytesRead += other.bytesRead
	s.directoryReadNS += other.directoryReadNS
	s.fileReadNS += other.fileReadNS
	s.parseNS += other.parseNS
}

func (s *configReadStats) record(phase *startup.Phase) {
	if phase == nil {
		return
	}
	phase.SetMetric("config_files.nested_directories_read", float64(s.directoriesRead))
	phase.SetMetric("config_files.nested_directory_errors", float64(s.directoryErrors))
	phase.SetMetric("config_files.files_attempted", float64(s.filesAttempted))
	phase.SetMetric("config_files.files_read", float64(s.filesRead))
	phase.SetMetric("config_files.read_errors", float64(s.readErrors))
	phase.SetMetric("config_files.empty_files", float64(s.emptyFiles))
	phase.SetMetric("config_files.configs_parsed", float64(s.configsParsed))
	phase.SetMetric("config_files.config_errors", float64(s.configErrors))
	phase.SetMetric("config_files.bytes_read", float64(s.bytesRead))
	phase.SetMetric("config_files.nested_directory_read_ns", float64(s.directoryReadNS))
	phase.SetMetric("config_files.file_read_ns", float64(s.fileReadNS))
	phase.SetMetric("config_files.parse_ns", float64(s.parseNS))
}
