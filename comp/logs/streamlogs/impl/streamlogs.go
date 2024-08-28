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

	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/api/api/utils/stream"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	coresetting "github.com/DataDog/datadog-agent/comp/core/settings"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	streamlogs "github.com/DataDog/datadog-agent/comp/logs/streamlogs/def"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	defaultLogFilePath = commonpath.DefaultStreamlogsLogFile
	streamLogDuration  = 60 * time.Second
)

// Requires defines the dependencies for the streamlogs component
type Requires struct {
	compdef.In
	LogAgent    optional.Option[logsAgent.Component]
	Config      config.Component
	CoreSetting coresetting.Component
}

// Provides defines the output of the streamlogs component
type Provides struct {
	compdef.Out

	Comp          streamlogs.Component
	FlareProvider flaretypes.Provider
}

// streamlog is a type that contains information needed to insert into a flare from the streamlog process.
type streamlogsimpl struct {
	logAgent    optional.Option[logsAgent.Component]
	config      config.Component
	coresetting coresetting.Component
}

// NewRCStreamLogFlare creates a new streamlogs for remote config flare component
func NewRCStreamLogFlare(reqs Requires) (Provides, error) {
	sl := &streamlogsimpl{
		logAgent:    reqs.LogAgent,
		config:      reqs.Config,
		coresetting: reqs.CoreSetting,
	}

	provides := Provides{
		FlareProvider: flaretypes.NewProvider(sl.fillFlare),
		Comp:          sl,
	}
	return provides, nil
}

// exportStreamLogsIfEnabled streams logs when runtime is enabled
func (sl *streamlogsimpl) exportStreamLogsIfEnabled(logAgent logsAgent.Component) error {
	// If the streamlog runtime setting is set, start streaming log to default file
	enableStreamLog, err := sl.coresetting.GetRuntimeSetting("enable_stream_logs")
	if err != nil {
		return err
	}

	if values, ok := enableStreamLog.([]interface{}); ok && len(values) > 1 {
		if enable, ok := values[1].(bool); ok && enable {
			streamLogParams := stream.LogParams{
				FilePath: defaultLogFilePath,
				Duration: streamLogDuration,
			}
			if err := stream.ExportStreamLogs(logAgent, &streamLogParams); err != nil {
				return fmt.Errorf("failed to export stream logs: %w", err)
			}
		}
	}
	return nil
}

func (sl *streamlogsimpl) fillFlare(fb flaretypes.FlareBuilder) error {
	streamlogsLogFile := sl.config.GetString("streamlogs_log_file")
	if streamlogsLogFile == "" {
		streamlogsLogFile = defaultLogFilePath
	}

	la, ok := sl.logAgent.Get()
	if !ok {
		return fmt.Errorf("log agent not found, unable to export stream logs")
	}

	if err := sl.exportStreamLogsIfEnabled(la); err != nil {
		return err
	}

	shouldIncludeFunc := func(path string) bool {
		return filepath.Ext(path) == ".log" || getFirstSuffix(path) == ".log"
	}

	if err := fb.CopyDirToWithoutScrubbing(filepath.Dir(streamlogsLogFile), "logs/streamlogs_info", shouldIncludeFunc); err != nil {
		return fmt.Errorf("failed to copy logs to flare: %w", err)
	}

	return nil
}

func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
}
