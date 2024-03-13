// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bytes"
	"io"

	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ChannelWriter is a buffered writer that log lines (lines ending with a \n) to a channel
// to be sent to our intake.
type ChannelWriter struct {
	Buffer  bytes.Buffer
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

// Write buffers Writes from our stdout/stderr fd,
// and sends to the channel once we've received newlines.
func (cw *ChannelWriter) Write(p []byte) (n int, err error) {
	n, err = cw.Buffer.Write(p)
	if err != nil {
		return n, err
	}

	for {
		line, err := cw.Buffer.ReadString('\n')
		if err == io.EOF {
			// If EOF, push the line back to buffer and wait for more data
			cw.Buffer.WriteString(line)
			break
		}
		if err != nil {
			return n, err
		}

		// This line is empty as it just contains a newline, we don't need to send it
		if len(line) <= 0 {
			continue
		}

		channelMessage := &logConfig.ChannelMessage{
			Content: []byte(line[:len(line)-1]),
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

	}
	return n, nil
}
