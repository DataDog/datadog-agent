// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bytes"
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
	sourceEnvVar        = "DD_SOURCE"
	sourceName          = "Datadog Agent"
)

// Config holds the log configuration
type Config struct {
	FlushTimeout time.Duration
	Channel      chan *logConfig.ChannelMessage
	source       string
	isEnabled    bool
}

// CustomWriter wraps the log config to allow stdout/stderr redirection
type CustomWriter struct {
	LogConfig    *Config
	LineBuffer   bytes.Buffer
	ShouldBuffer bool
	IsError      bool
}

// CreateConfig builds and returns a log config
func CreateConfig(origin string) *Config {
	var source string
	if source = strings.ToLower(os.Getenv(sourceEnvVar)); source == "" {
		source = origin
	}
	return &Config{
		FlushTimeout: defaultFlushTimeout,
		Channel:      make(chan *logConfig.ChannelMessage),
		source:       source,
		isEnabled:    isEnabled(os.Getenv(logEnabledEnvVar)),
	}
}

// SetupLog creates the log agent and sets the base tags
func SetupLog(conf *Config, tags map[string]string) logsAgent.ServerlessLogsAgent {
	logsAgent, _ := serverlessLogs.SetupLogAgent(conf.Channel, sourceName, conf.source)
	serverlessLogs.SetLogsTags(tag.GetBaseTagsArrayWithMetadataTags(tags))
	return logsAgent
}

func isEnabled(envValue string) bool {
	return strings.ToLower(envValue) == "true"
}
