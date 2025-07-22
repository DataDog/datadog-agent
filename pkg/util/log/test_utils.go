// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package log

import (
	"context"
	"log/slog"
)

// BasicTestFormatter returns a formatter that logs the level, function name, and message.
// It is used for testing purposes.
func BasicTestFormatter() func(context.Context, slog.Record) string {
	return TemplateFormatter("[{{.LEVEL}}] {{.FuncShort}}: {{.msg}}\n")
}
