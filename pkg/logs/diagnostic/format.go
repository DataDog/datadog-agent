// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnostic

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// Formatter defines how a particular message.Message should be formatted
type Formatter interface {
	// Format transforms the supplied message.Message into a more user-friendly format, for diagnostic purposes.
	Format(*message.Message, string, []byte) string
}

// logFormatter is the default Formatter which supports transforming log pipeline messages into a more useful format.
type logFormatter struct{}

func (l *logFormatter) Format(m *message.Message, eventType string, redactedMsg []byte) string {
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		hname = "unknown"
	}

	ts := time.Now().UTC()
	// TODO(remy): should we consider renaming the "Timestamp: %s" to mention
	// it's only concerning the serverless agent?
	if !m.ServerlessExtra.Timestamp.IsZero() {
		ts = m.ServerlessExtra.Timestamp
	}

	return fmt.Sprintf("Integration Name: %s | Type: %s | Status: %s | Timestamp: %s | Hostname: %s | Service: %s | Source: %s | Tags: %s | Message: %s\n",
		m.Origin.LogSource.Name,
		m.Origin.LogSource.Config.Type,
		m.GetStatus(),
		ts,
		hname,
		m.Origin.Service(),
		m.Origin.Source(),
		m.Origin.TagsToString(),
		string(redactedMsg))
}
