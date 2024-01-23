// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnostic

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Formatter defines how a particular message.Message should be formatted
type Formatter interface {
	// Format transforms the supplied message.Message into a more user-friendly format, for diagnostic purposes.
	Format(*message.Message, string, []byte) string
}

// logFormatter is the default Formatter which supports transforming log pipeline messages into a more useful format.
type logFormatter struct{}

//nolint:revive // TODO(AML) Fix revive linter
func (l *logFormatter) Format(m *message.Message, eventType string, redactedMsg []byte) string {
	panic("not called")
}
