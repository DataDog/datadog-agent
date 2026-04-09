// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import "github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers"

// globalCapture is the capture handler registered during logger setup.
// It is accessed by the log component's DrainErrorLogs() method.
var globalCapture *handlers.Capture

// RegisterCaptureHandler stores the capture handler so that DrainCapturedLogs
// can be called later. Called once during SetupLogger.
func RegisterCaptureHandler(h *handlers.Capture) {
	globalCapture = h
}

// DrainCapturedLogs returns and clears all buffered Error/Critical log entries
// since the last call. Returns nil if no capture handler has been registered.
func DrainCapturedLogs() []handlers.CapturedLog {
	if globalCapture == nil {
		return nil
	}
	return globalCapture.Drain()
}
