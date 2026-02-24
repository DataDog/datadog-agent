// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package streamlogs provides diagnostic log formatting for serverless environments.
package streamlogs

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Formatter formats diagnostic logs for serverless environments.
type Formatter struct{}

// Format formats diagnostic log messages with metadata.
func (Formatter) Format(m *message.Message, _ string, redactedMsg []byte) string {
	if m == nil {
		return ""
	}
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
