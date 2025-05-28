// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ChannelWriter is a buffered writer that sends log messages to a channel
// to be sent to our intake.
type ChannelWriter struct {
	Channel chan *logConfig.ChannelMessage
	IsError bool
}

// NewChannelWriter returns a new channel writer.
// Implements io.Writer, used for redirecting stdout/stderr
// logs to Datadog.
func NewChannelWriter(ch chan *logConfig.ChannelMessage, isError bool) *ChannelWriter {
	return &ChannelWriter{
		Channel: ch,
		IsError: isError,
	}
}

// Write processes writes from our stdout/stderr fd and sends complete
// log messages to the channel.
func (cw *ChannelWriter) Write(p []byte) (n int, err error) {
	channelMessage := &logConfig.ChannelMessage{
		Content: p,
		IsError: cw.IsError,
	}

	select {
	case cw.Channel <- channelMessage:
		// Success case -- the channel isn't full, and can accommodate our message
	default:
		// Channel is full (i.e, we aren't flushing data to Datadog as our backend is down).
		// message will be dropped.
		log.Debug("Log dropped due to full buffer")
	}
	return len(p), nil
}
