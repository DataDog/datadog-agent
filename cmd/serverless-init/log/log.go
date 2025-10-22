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
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

const (
	defaultFlushTimeout   = 5 * time.Second
	defaultTailingPath    = "/home/LogFiles/*$COMPUTERNAME*.log"
	modifiableTailingPath = "/home/LogFiles/*$COMPUTERNAME*%s.log"
	logEnabledEnvVar      = "DD_LOGS_ENABLED"
	envVarTailFilePath    = "DD_SERVERLESS_LOG_PATH"
	aasInstanceTailing    = "DD_AAS_INSTANCE_LOGGING_ENABLED"
	aasLoggingDescriptor  = "DD_AAS_INSTANCE_LOG_FILE_DESCRIPTOR"
	sourceEnvVar          = "DD_SOURCE"
	sourceName            = "Datadog Agent"
	// Log files are persisted between instance restarts in AAS, so we want to start tailing logs from the end to prevent duplicates.
	// For ephemeral environments (Cloud Run, Azure Container Apps, etc.), we also want "end".
	tailingMode = "end"
)

// Config holds the log configuration
type Config struct {
	FlushTimeout time.Duration
	Channel      chan *logConfig.ChannelMessage
	source       string
	IsEnabled    bool
}

// CreateConfig builds and returns a log config
func CreateConfig(defaultSource string) *Config {
	var source string
	if source = strings.ToLower(os.Getenv(sourceEnvVar)); source == "" {
		source = defaultSource
	}
	return &Config{
		FlushTimeout: defaultFlushTimeout,
		// Use a buffered channel with size 10000
		Channel:   make(chan *logConfig.ChannelMessage, 10000),
		source:    source,
		IsEnabled: isEnabled(os.Getenv(logEnabledEnvVar)),
	}
}

// SetupLogAgent creates the log agent and sets the base tags
func SetupLogAgent(conf *Config, tags map[string]string, tagger tagger.Component, compression logscompression.Component, hostname hostnameinterface.Component, origin string) logsAgent.ServerlessLogsAgent {
	logsAgent, _ := serverlessLogs.SetupLogAgent(conf.Channel, sourceName, conf.source, tagger, compression, hostname)

	tagsArray := serverlessTag.MapToArray(tags)

	addFileTailing(logsAgent, conf.source, tagsArray, origin)

	serverlessLogs.SetLogsTags(tagsArray)
	return logsAgent
}

func addFileTailing(logsAgent logsAgent.ServerlessLogsAgent, source string, tags []string, origin string) {
	appServiceDefaultLoggingEnabled := origin == "appservice" && isInstanceTailingEnabled()
	// The Azure App Service log volume is shared across all instances. This leads to every instance tailing the same files.
	// To avoid this, we want to add the azure instance ID to the filepath so each instance tails their respective system log files.
	// Users can also add $COMPUTERNAME to their custom files to achieve the same result.
	if appServiceDefaultLoggingEnabled {
		src := sources.NewLogSource("aas-instance-file-tail", &logConfig.LogsConfig{
			Type:        logConfig.FileType,
			Path:        setAasInstanceTailingPath(),
			TailingMode: tailingMode,
			Service:     os.Getenv("DD_SERVICE"),
			Tags:        tags,
			Source:      source,
		})
		logsAgent.GetSources().AddSource(src)
		// If we are not in Azure or the aas instance env var is not set, we fall back to the previous behavior
	} else if filePath, set := os.LookupEnv(envVarTailFilePath); set {
		src := sources.NewLogSource("serverless-file-tail", &logConfig.LogsConfig{
			Type:        logConfig.FileType,
			Path:        filePath,
			TailingMode: tailingMode,
			Service:     os.Getenv("DD_SERVICE"),
			Tags:        tags,
			Source:      source,
		})
		logsAgent.GetSources().AddSource(src)
	}
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
