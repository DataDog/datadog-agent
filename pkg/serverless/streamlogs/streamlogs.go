// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package streamlogs package is responsible for stream-logs on the serverless environment.
package streamlogs

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	streamLogsEnvvar = "DD_SERVERLESS_STREAM_LOGS"
	streamLogsPrefix = "DD_EXTENSION | stream-logs |"
)

func isEnabled() bool {
	v := os.Getenv(streamLogsEnvvar)
	return v == "true"
}

// Is returns true if the line is a stream-logs line.
func Is(line string) bool {
	return strings.HasPrefix(line, streamLogsPrefix)
}

// Formatter formats the stream-logs.
type Formatter struct{}

// Format formats the stream-logs.
func (Formatter) Format(m *message.Message, _ string, redactedMsg []byte) string {
	ts := m.ServerlessExtra.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	return fmt.Sprintf("Integration Name: %s | Type: %s | Status: %s | Timestamp: %s | Service: %s | Source: %s | Tags: %s | Message: %s\n",
		m.Origin.LogSource.Name,
		m.Origin.LogSource.Config.Type,
		m.GetStatus(),
		ts,
		m.Origin.Service(),
		m.Origin.Source(),
		m.TagsToString(),
		string(redactedMsg))
}

type messageReceiverGetter interface {
	GetMessageReceiver() *diagnostic.BufferedMessageReceiver
}

// Run stream logs to io.Writer.
func Run(ctx context.Context, mrg messageReceiverGetter, w io.Writer) {
	if !isEnabled() {
		return
	}
	mr := mrg.GetMessageReceiver()
	if mr == nil {
		log.Error("Cannot run stream-logs: message receiver is nil")
		return
	}
	if !mr.SetEnabled(true) {
		log.Error("Cannot run stream-logs: message receiver is already streaming logs")
		return
	}
	defer mr.SetEnabled(false)
	// Fields to be filtered are single kind of string in the serverless environment.
	for line := range mr.Filter(nil, ctx.Done()) {
		fmt.Fprintf(w, "%s %s", streamLogsPrefix, line)
	}
}
