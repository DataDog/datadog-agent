// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultFlushTimeout = 5 * time.Second
	logEnabledEnvVar    = "DD_LOGS_ENABLED"
	sourceEnvVar        = "DD_SOURCE"
	sourceName          = "Datadog Agent"
	maxBufferSize       = 256 * 1024 // Max log size is 256KB: https://docs.datadoghq.com/agent/logs/log_transport/?tab=https
)

// Config holds the log configuration
type Config struct {
	FlushTimeout time.Duration
	channel      chan *logConfig.ChannelMessage
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
		channel:      make(chan *logConfig.ChannelMessage),
		source:       source,
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
func SetupLog(conf *Config, tags map[string]string) logsAgent.ServerlessLogsAgent {
	logsAgent, _ := serverlessLogs.SetupLogAgent(conf.channel, sourceName, conf.source)
	serverlessLogs.SetLogsTags(tag.GetBaseTagsArrayWithMetadataTags(tags))
	return logsAgent
}

func (cw *CustomWriter) Write(p []byte) (n int, err error) {
	return cw.writeWithMaxBufferSize(p, maxBufferSize)
}

func (cw *CustomWriter) writeWithMaxBufferSize(p []byte, maxBufferSize int) (n int, err error) {
	fmt.Print(string(p))

	if len(p) > maxBufferSize {
		log.Errorf("Received a log chunk over %d kb. Truncating", maxBufferSize)
		p = p[:maxBufferSize]
	}

	if !cw.ShouldBuffer {
		Write(cw.LogConfig, p, cw.IsError)
		return len(p), nil
	}

	// Prevent buffer overflow, flush the buffer if writing the current chunk
	// will exceed maxBufferSize
	if cw.LineBuffer.Len()+len(p) > maxBufferSize {
		log.Errorf("Log buffer exceeds %d kb. Flushing log buffer", maxBufferSize)
		Write(cw.LogConfig, getByteArrayClone(cw.LineBuffer.Bytes()), cw.IsError)
		cw.LineBuffer.Reset()
	}

	// Only flush the log buffer if the chunk to be appended ends in a newline.
	// Otherwise, the chunk only represents part of a log. Push it into the buffer and wait
	// for the rest of the log before flushing.
	cw.LineBuffer.Write(p)
	if string(p[len(p)-1]) != "\n" {
		return len(p), nil
	}

	Write(cw.LogConfig, getByteArrayClone(cw.LineBuffer.Bytes()), cw.IsError)
	cw.LineBuffer.Reset()

	return len(p), nil
}

func getByteArrayClone(src []byte) []byte {
	clone := make([]byte, len(src))
	copy(clone, src)
	return clone
}

func isEnabled(envValue string) bool {
	return strings.ToLower(envValue) == "true"
}
