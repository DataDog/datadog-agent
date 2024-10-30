// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package streamlogsimpl implements the streamlogs component interface
package streamlogsimpl

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/api/api/utils/stream"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	logger "github.com/DataDog/datadog-agent/comp/core/log/def"
	coresetting "github.com/DataDog/datadog-agent/comp/core/settings"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Requires defines the dependencies for the streamlogs component
type Requires struct {
	compdef.In
	LogsAgent   optional.Option[logsAgent.Component]
	Logger      logger.Component
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
	logsAgent   logsAgent.Component
	logger      logger.Component
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
	la, ok := reqs.LogsAgent.Get()
	if !ok {
		reqs.Logger.Debug("logs agent is not enabled. unable to export stream logs in any flare")
		return Provides{}, nil
	}

	sl := &streamlogsimpl{
		logsAgent:   la,
		logger:      reqs.Logger,
		config:      reqs.Config,
		coresetting: reqs.CoreSetting,
	}

	provides := Provides{
		FlareProvider: flaretypes.NewProvider(sl.fillFlare),
	}
	return provides, nil
}

// exportStreamLogs export output of stream-logs to a file. Currently used for remote config stream logs
func exportStreamLogs(la logsAgent.Component, logger logger.Component, streamLogParams *LogParams) error {
	if err := stream.EnsureDirExists(streamLogParams.FilePath); err != nil {
		return fmt.Errorf("error creating directory for file %q: %w", streamLogParams.FilePath, err)
	}
	f, bufWriter, err := stream.OpenFileForWriting(streamLogParams.FilePath)
	if err != nil {
		return fmt.Errorf("error opening file %s for writing: %v", streamLogParams.FilePath, err)
	}
	defer func() {
		if err = bufWriter.Flush(); err != nil {
			logger.Errorf("Error flushing buffer for log stream: %v", err)

		}
		if err = f.Close(); err != nil {
			logger.Errorf("Error closing file for log stream: %v", err)
		}
	}()

	messageReceiver := la.GetMessageReceiver()

	if !messageReceiver.SetEnabled(true) {
		return errors.New("unable to enable message receiver, another client is already streaming logs")
	}
	defer messageReceiver.SetEnabled(false)

	done := make(chan struct{})

	logChan := messageReceiver.Filter(nil, done)

	timer := time.NewTimer(streamLogParams.Duration)
	defer timer.Stop()
	var wg sync.WaitGroup

	time.AfterFunc(streamLogParams.Duration, func() {
		wg.Wait()
		close(done)
	})

	for {
		log, ok := <-logChan
		if !ok {
			return nil
		}

		wg.Add(1)
		go func(log string) {
			defer wg.Done()
			if _, err := bufWriter.WriteString(log + "\n"); err != nil {
				logger.Errorf("failed to write to file: %v", err)
			}
		}(log)
	}
}

// exportStreamLogsIfEnabled streams logs when runtime is enabled
func (sl *streamlogsimpl) exportStreamLogsIfEnabled(logsAgent logsAgent.Component, streamlogsLogFilePath string) error {
	// If the enable_streamlogs config is set, start streaming log to default file
	enabled := pkgconfigsetup.Datadog().GetBool("logs_config.streaming.enable_streamlogs")

	if enabled {
		streamLogParams := LogParams{
			FilePath: streamlogsLogFilePath,
			Duration: 60 * time.Second, // Default duration is 60 seconds
		}
		if err := exportStreamLogs(logsAgent, sl.logger, &streamLogParams); err != nil {
			return fmt.Errorf("failed to export stream logs: %w", err)
		}
	}
	return nil
}

func (sl *streamlogsimpl) fillFlare(fb flaretypes.FlareBuilder) error {
	streamlogsLogFile := sl.config.GetString("logs_config.streaming.streamlogs_log_file")

	if err := sl.exportStreamLogsIfEnabled(sl.logsAgent, streamlogsLogFile); err != nil {
		return err
	}

	// shouldIncludeFunc ensures that only correct .log files are collected from the streamlogs_info folder
	shouldIncludeFunc := func(path string) bool {
		return filepath.Ext(path) == ".log"
	}

	if err := fb.CopyDirTo(filepath.Dir(streamlogsLogFile), "logs/streamlogs_info", shouldIncludeFunc); err != nil {
		return fmt.Errorf("failed to copy logs to flare: %w", err)
	}

	return nil
}
