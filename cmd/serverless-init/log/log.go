// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	"github.com/DataDog/datadog-agent/pkg/config"
	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultFlushTimeout = 5 * time.Second
	loggerName          = "DD_LOG_AGENT"
	logLevelEnvVar      = "DD_LOG_LEVEL"
	logEnabledEnvVar    = "DD_LOGS_ENABLED"
	sourceEnvVar        = "DD_SOURCE"
	sourceName          = "Datadog Agent"
)

// Config holds the log configuration
type Config struct {
	FlushTimeout time.Duration
	channel      chan *logConfig.ChannelMessage
	source       string
	loggerName   config.LoggerName
	isEnabled    bool
}

// CustomWriter wraps the log config to allow stdout/stderr redirection
type CustomWriter struct {
	LogConfig  *Config
	LineBuffer bytes.Buffer
	IsError    bool
}

// CreateConfig builds and returns a log config
func CreateConfig(origin string) *Config {
	var source string
	if source = strings.ToLower(os.Getenv(sourceEnvVar)); source == "" {
		source = origin
	}
	return &Config{
		FlushTimeout: defaultFlushTimeout,
		channel:      make(chan *logConfig.ChannelMessage),
		source:       source,
		loggerName:   loggerName,
		isEnabled:    isEnabled(os.Getenv(logEnabledEnvVar)),
	}
}

// Write writes the log message to the log message channel for processing
func Write(conf *Config, msgToSend []byte, isError bool) {
	if conf.isEnabled {
		logMessage := &logConfig.ChannelMessage{
			Content: msgToSend,
			IsError: isError,
		}
		conf.channel <- logMessage
	}
}

// SetupLog creates the log agent and sets the base tags
func SetupLog(conf *Config, tags map[string]string) {
	if err := config.SetupLogger(
		conf.loggerName,
		"error", // will be re-set later with the value from the env var
		"",      // logFile -> by setting this to an empty string, we don't write the logs to any file
		"",      // syslog URI
		false,   // syslog_rfc
		true,    // log_to_console
		false,   // log_format_json
	); err != nil {
		log.Errorf("Unable to setup logger: %s", err)
	}

	if logLevel := os.Getenv(logLevelEnvVar); len(logLevel) > 0 {
		if err := config.ChangeLogLevel(logLevel); err != nil {
			log.Errorf("Unable to change the log level: %s", err)
		}
	}
	serverlessLogs.SetupLogAgent(conf.channel, sourceName, conf.source)
	serverlessLogs.SetLogsTags(tag.GetBaseTagsArrayWithMetadataTags(tags))
}

func (cw *CustomWriter) Write(p []byte) (n int, err error) {
	fmt.Print(string(p))
	cw.LineBuffer.Write(p)
	scanner := bufio.NewScanner(&cw.LineBuffer)
	for scanner.Scan() {
		logLine := scanner.Bytes()
		// Don't write anything if we don't actually have a message.
		// This can happen in the case of consecutive newlines.
		if len(logLine) == 0 {
			continue
		}
		Write(cw.LogConfig, logLine, cw.IsError)
	}
	return len(p), nil
}

func isEnabled(envValue string) bool {
	return strings.ToLower(envValue) == "true"
}
