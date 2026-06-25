// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"log/slog"
)

// NewDisabled returns a handler which never writes anything.
func NewDisabled() slog.Handler {
	return slog.DiscardHandler
}
