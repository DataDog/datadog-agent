// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnostic

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Formatter defines how a particular message.Message should be formatted
type Formatter interface {
	// Format transforms the supplied message.Message into a more user-friendly format, for diagnostic purposes.
	Format(*message.Message, string, []byte) string
}

// logFormatter is the default Formatter which supports transforming log pipeline messages into a more useful format.
type logFormatter struct {
	hostname hostnameinterface.Component
}

func (l *logFormatter) Format(m *message.Message, _ string, redactedMsg []byte) string {
	hname, err := l.hostname.Get(context.TODO())
	if err != nil {
		hname = "unknown"
	}

	return fmt.Sprintf("Integration Name: %s | Type: %s | Status: %s | Timestamp: %s | Hostname: %s | Service: %s | Source: %s | Tags: %s | Message: %s\n",
		m.Origin.LogSource.Name,
		m.Origin.LogSource.Config.Type,
		m.GetStatus(),
		time.Now().UTC(),
		hname,
		m.Origin.Service(),
		m.Origin.Source(),
		m.TagsToString(),
		string(redactedMsg))
}
