// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log is responsible for settings around logging output from customer functions
// to be sent to Datadog (logs monitoring product).
// It does *NOT* control the internal debug logging of the agent.
package log

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

const (
	defaultTailingPath    = "/home/LogFiles/*$COMPUTERNAME*.log"
	modifiableTailingPath = "/home/LogFiles/*$COMPUTERNAME*%s.log"
	logEnabledEnvVar      = "DD_LOGS_ENABLED"
	envVarTailFilePath    = "DD_SERVERLESS_LOG_PATH"
	aasInstanceTailing    = "DD_AAS_INSTANCE_LOGGING_ENABLED"
	aasLoggingDescriptor  = "DD_AAS_INSTANCE_LOG_FILE_DESCRIPTOR"
	sourceEnvVar          = "DD_SOURCE"
	sourceName            = "Datadog Agent"
	// The registry auditor persists each tailed file's offset to
	// registry.json under logs_config.run_path (see cmd/serverless-init/main.go
	// preloadEarly), and a persisted offset always wins over tailingMode
	// (pkg/logs/launchers/file/position.go). So "beginning" only takes effect
	// when there is no registry entry yet: a fresh Cloud Run/Container Apps
	// instance, or the first AAS start on a given log file. That's exactly the
	// cold-start case we want to capture the app's startup line for. A restart
	// within the same instance resumes from the persisted offset instead of
	// re-reading, so this is also safe for AAS's persistent log files -
	// provided run_path persists alongside the logs, which preloadEarly
	// guarantees.
	tailingMode = "beginning"
)

// AASPersistentLogDir is the directory portion of defaultTailingPath: Azure
// App Service persists /home across instance restarts and scale events (an
// Azure Files-backed share), so cmd/serverless-init/main.go's preloadEarly
// defaults logs_config.run_path here on AAS - putting the file-tailer
// registry on the same persistent volume as the logs it tracks, which
// tailingMode "beginning" requires to be safe on AAS.
var AASPersistentLogDir = path.Dir(defaultTailingPath)

// Config holds the log configuration
type Config struct {
	FlushTimeout time.Duration
	Channel      chan *logConfig.ChannelMessage
	source       string
	IsEnabled    bool
}

// CreateConfig builds and returns a log config. flushTimeout bounds the
// flush duration when stopping the logs agent at shutdown.
func CreateConfig(defaultSource string, flushTimeout time.Duration) *Config {
	var source string
	if source = strings.ToLower(os.Getenv(sourceEnvVar)); source == "" {
		source = defaultSource
	}
	return &Config{
		FlushTimeout: flushTimeout,
		// Use a buffered channel with size 10000
		Channel:   make(chan *logConfig.ChannelMessage, 10000),
		source:    source,
		IsEnabled: isEnabled(os.Getenv(logEnabledEnvVar)),
	}
}

// SetupLogAgent creates the log agent and sets the base tags
func SetupLogAgent(conf *Config, tags map[string]string, tagger tagger.Component, compression logscompression.Component, hostname hostnameinterface.Component, origin string) logsAgent.ServerlessLogsAgent {
	// serverless-init persists tailer offsets across restarts (see tailingMode
	// above), so it always wants the registry auditor.
	logsAgent, _ := serverlessLogs.SetupLogAgent(conf.Channel, sourceName, conf.source, tagger, compression, hostname, true)

	tagsArray := serverlessTag.MapToArray(tags)

	if src := createFileTailingSource(conf.source, tagsArray, origin); src != nil {
		logsAgent.GetSources().AddSource(src)
	}

	serverlessLogs.SetLogsTags(tagsArray)
	return logsAgent
}

// createFileTailingSource creates a log source for file tailing based on origin and environment
func createFileTailingSource(source string, tags []string, origin string) *sources.LogSource {
	appServiceDefaultLoggingEnabled := origin == "appservice" && isInstanceTailingEnabled()

	// The Azure App Service log volume is shared across all instances. This leads to every instance tailing the same files.
	// To avoid this, we want to add the azure instance ID to the filepath so each instance tails their respective system log files.
	// Users can also add $COMPUTERNAME to their custom files to achieve the same result.
	if appServiceDefaultLoggingEnabled {
		return sources.NewLogSource("aas-instance-file-tail", &logConfig.LogsConfig{
			Type:        logConfig.FileType,
			Path:        setAasInstanceTailingPath(),
			TailingMode: tailingMode,
			Service:     os.Getenv("DD_SERVICE"),
			Tags:        tags,
			Source:      source,
		})
	} else if filePath, set := os.LookupEnv(envVarTailFilePath); set {
		return sources.NewLogSource("serverless-file-tail", &logConfig.LogsConfig{
			Type:        logConfig.FileType,
			Path:        filePath,
			TailingMode: tailingMode,
			Service:     os.Getenv("DD_SERVICE"),
			Tags:        tags,
			Source:      source,
		})
	}
	return nil
}

func isEnabled(envValue string) bool {
	return strings.ToLower(envValue) == "true"
}

func isInstanceTailingEnabled() bool {
	val := strings.ToLower(os.Getenv(aasInstanceTailing))
	return val == "true" || val == "1"
}

func setAasInstanceTailingPath() string {
	customPath, set := os.LookupEnv(aasLoggingDescriptor)
	if set && customPath != "" {
		return os.ExpandEnv(fmt.Sprintf(modifiableTailingPath, customPath))
	}
	return os.ExpandEnv(defaultTailingPath)
}
