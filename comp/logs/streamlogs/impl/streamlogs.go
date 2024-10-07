// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package streamlogsimpl implements the streamlogs component interface
package streamlogsimpl

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/api/api/utils/stream"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	coresetting "github.com/DataDog/datadog-agent/comp/core/settings"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Requires defines the dependencies for the streamlogs component
type Requires struct {
	compdef.In
	LogsAgent   optional.Option[logsAgent.Component]
	Config      config.Component
	CoreSetting coresetting.Component
}

// Provides defines the output of the streamlogs component
type Provides struct {
	compdef.Out

	FlareProvider flaretypes.Provider
}

// streamlog is a type that contains information needed to insert into a flare from the streamlog process.
type streamlogsimpl struct {
	logsAgent   optional.Option[logsAgent.Component]
	config      config.Component
	coresetting coresetting.Component
}

// LogParams represents the parameters for streaming logs
type LogParams struct {
	// FilePath represents the output file path to write the log stream to.
	FilePath string

	// Duration represents the duration of the log stream.
	Duration time.Duration
}

// NewComponent creates a new streamlogs component for remote config flare component
func NewComponent(reqs Requires) (Provides, error) {
	sl := &streamlogsimpl{
		logsAgent:   reqs.LogsAgent,
		config:      reqs.Config,
		coresetting: reqs.CoreSetting,
	}

	provides := Provides{
		FlareProvider: flaretypes.NewProvider(sl.fillFlare),
	}
	return provides, nil
}

// exportStreamLogs export output of stream-logs to a file. Currently used for remote config stream logs
func exportStreamLogs(la logsAgent.Component, streamLogParams *LogParams) error {
	if err := stream.EnsureDirExists(streamLogParams.FilePath); err != nil {
		return fmt.Errorf("error creating directory for file %s: %v", streamLogParams.FilePath, err)
	}

	f, bufWriter, err := stream.OpenFileForWriting(streamLogParams.FilePath)
	if err != nil {
		return fmt.Errorf("error opening file %s for writing: %v", streamLogParams.FilePath, err)
	}
	defer func() {
		if err = bufWriter.Flush(); err != nil {
			log.Errorf("Error flushing buffer for log stream: %v", err)
		}
		if err = f.Close(); err != nil {
			log.Errorf("Error closing file for log stream: %v", err)
		}
	}()

	messageReceiver := la.GetMessageReceiver()

	if !messageReceiver.SetEnabled(true) {
		return fmt.Errorf("unable to enable message receiver, another client is already streaming logs")
	}
	defer messageReceiver.SetEnabled(false)

	var filters diagnostic.Filters
	done := make(chan struct{})
	defer close(done)

	logChan := messageReceiver.Filter(&filters, done)

	timer := time.NewTimer(streamLogParams.Duration)
	defer timer.Stop()

	for {
		select {
		case log := <-logChan:
			if _, err := bufWriter.WriteString(log + "\n"); err != nil {
				return fmt.Errorf("failed to write to file: %v", err)
			}
		case <-timer.C:
			return nil
		}
	}
}

// exportStreamLogsIfEnabled streams logs when runtime is enabled
func (sl *streamlogsimpl) exportStreamLogsIfEnabled(logsAgent logsAgent.Component, streamlogsLogFilePath string) error {
	// If the streamlog runtime setting is set, start streaming log to default file
	enableStreamLog, err := sl.coresetting.GetRuntimeSetting("enable_stream_logs")
	if err != nil {
		return err
	}

	if values, ok := enableStreamLog.([]interface{}); ok && len(values) > 1 {
		if enable, ok := values[1].(bool); ok && enable {
			streamLogParams := LogParams{
				FilePath: streamlogsLogFilePath,
				Duration: 60 * time.Second, // Default duration is 60 seconds
			}
			if err := exportStreamLogs(logsAgent, &streamLogParams); err != nil {
				return fmt.Errorf("failed to export stream logs: %w", err)
			}
		}
	}
	return nil
}

func (sl *streamlogsimpl) fillFlare(fb flaretypes.FlareBuilder) error {
	streamlogsLogFile := sl.config.GetString("logs_config.streaming.streamlogs_log_file")

	la, ok := sl.logsAgent.Get()
	if !ok {
		return fmt.Errorf("log agent not found, unable to export stream logs")
	}

	if err := sl.exportStreamLogsIfEnabled(la, streamlogsLogFile); err != nil {
		return err
	}

	// shouldIncludeFunc ensures that only correct extension/suffix log files are collected from the streamlogs_info folder
	// This include log roll over files eg: streamlogs.log.1 and to exclude non log files that might be present in the folder.
	shouldIncludeFunc := func(path string) bool {
		return filepath.Ext(path) == ".log" || getFirstSuffix(path) == ".log"
	}

	if err := fb.CopyDirTo(filepath.Dir(streamlogsLogFile), "logs/streamlogs_info", shouldIncludeFunc); err != nil {
		return fmt.Errorf("failed to copy logs to flare: %w", err)
	}

	return nil
}

// getFirstSuffix returns the first suffix of a file name (e.g., for "stream.error.log", it returns ".error")
func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
}
