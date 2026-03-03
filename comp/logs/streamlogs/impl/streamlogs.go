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
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	logger "github.com/DataDog/datadog-agent/comp/core/log/def"
	coresetting "github.com/DataDog/datadog-agent/comp/core/settings"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies for the streamlogs component
type Requires struct {
	compdef.In
	LogsAgent   option.Option[logsAgent.Component]
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
		FlareProvider: flaretypes.NewProviderWithTimeout(sl.fillFlare, sl.getFlareTimeout),
	}
	return provides, nil
}

// exportStreamLogs export output of stream-logs to a file. Currently used for remote config stream logs
func exportStreamLogs(la logsAgent.Component, logger logger.Component, streamLogParams *LogParams) error {
	fp := streamLogParams.FilePath
	if err := filesystem.EnsureParentDirsExist(fp); err != nil {
		return fmt.Errorf("error creating directory for file %q: %w", fp, err)
	}
	logger.Infof("Opening file %s for writing logs. This file will be used to store streamlog output.", fp)
	f, bufWriter, err := filesystem.OpenFileForWriting(fp)
	if err != nil {
		return fmt.Errorf("error opening file %s for writing: %v", fp, err)
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

	droppedLogs := 0
	time.AfterFunc(streamLogParams.Duration, func() {
		close(done)
	})

	for {
		log, ok := <-logChan
		if !ok {
			break
		}
		if _, err := bufWriter.WriteString(log); err != nil {
			droppedLogs++
			logger.Errorf("failed to write to file: %v", err)
		}
	}

	if droppedLogs > 0 {
		logger.Infof("Dropped %d logs from streamlogs", droppedLogs)
	}

	return nil
}

// exportStreamLogsIfEnabled streams logs to a file if the enable_streamlogs config is set
func (sl *streamlogsimpl) exportStreamLogsIfEnabled(logsAgent logsAgent.Component, streamlogsLogFilePath string, fb flaretypes.FlareBuilder) error {

	slDuration := fb.GetFlareArgs().StreamLogsDuration
	if slDuration <= 0 {
		return errors.New("remote streamlogs has been disabled via an unset duration, exiting streamlogs flare filler")
	}
	streamLogParams := LogParams{
		FilePath: streamlogsLogFilePath,
		Duration: slDuration,
	}
	if err := exportStreamLogs(logsAgent, sl.logger, &streamLogParams); err != nil {
		return fmt.Errorf("failed to export stream logs: %w", err)
	}
	return nil
}

// Currently flare args are only populated (and this function is only enabled) via
// the RC flare generation flow. The goal is to shift other flare generation flows
// to utilize this provider over time, which will require additional plumbing.
func (sl *streamlogsimpl) fillFlare(fb flaretypes.FlareBuilder) error {
	streamlogsLogFile := sl.config.GetString("logs_config.streaming.streamlogs_log_file")

	if err := sl.exportStreamLogsIfEnabled(sl.logsAgent, streamlogsLogFile, fb); err != nil {
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

// getFlareTimeout returns the timeout for the flare when streaming logs.
func (sl *streamlogsimpl) getFlareTimeout(fb flaretypes.FlareBuilder) time.Duration {
	// Base timeout is the default duration for streaming logs
	baseTimeout := fb.GetFlareArgs().StreamLogsDuration

	// overhead is the duration for processing file operations (e.g., copying logs to the flare)
	overhead := 10 * time.Second

	// Total timeout
	return baseTimeout + overhead
}
