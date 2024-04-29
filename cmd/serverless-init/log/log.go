// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log is responsible for settings around logging output from customer functions
// to be sent to Datadog (logs monitoring product).
// It does *NOT* control the internal debug logging of the agent.
package log

import (
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
)

const (
	defaultFlushTimeout = 5 * time.Second
	logEnabledEnvVar    = "DD_LOGS_ENABLED"
	envVarTailFilePath  = "DD_SERVERLESS_LOG_PATH"
	sourceEnvVar        = "DD_SOURCE"
	sourceName          = "Datadog Agent"
)

// Config holds the log configuration
type Config struct {
	FlushTimeout time.Duration
	Channel      chan *logConfig.ChannelMessage
	source       string
	IsEnabled    bool
}

// CreateConfig builds and returns a log config
func CreateConfig(origin string) *Config {
	var source string
	if source = strings.ToLower(os.Getenv(sourceEnvVar)); source == "" {
		source = origin
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
func SetupLogAgent(conf *Config, tags map[string]string) logsAgent.ServerlessLogsAgent {
	logsAgent, _ := serverlessLogs.SetupLogAgent(conf.Channel, sourceName, conf.source)

	tagsArray := tag.GetBaseTagsArrayWithMetadataTags(tags)

	addFileTailing(logsAgent, tagsArray)

	serverlessLogs.SetLogsTags(tagsArray)
	return logsAgent
}

func addFileTailing(logsAgent logsAgent.ServerlessLogsAgent, tags []string) {
	if filePath, set := os.LookupEnv(envVarTailFilePath); set {
		src := sources.NewLogSource("serverless-file-tail", &logConfig.LogsConfig{
			Type:    logConfig.FileType,
			Path:    filePath,
			Service: os.Getenv("DD_SERVICE"),
			Tags:    tags,
		})
		logsAgent.GetSources().AddSource(src)
	}
}

func isEnabled(envValue string) bool {
	return strings.ToLower(envValue) == "true"
}
