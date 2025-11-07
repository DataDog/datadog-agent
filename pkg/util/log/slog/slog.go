// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package slog provides a slog-based implementation of the log package.
package slog

import (
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers"
)

// Default returns a default logger
func Default() *Wrapper {
	// the default seelog logger is an asynchronous loop logger, prints to stdout,
	// with format "%Ns [%Level] %Msg%n"
	return nil //TODO: implement
}

// Disabled returns a disabled logger
func Disabled() *Wrapper {
	disabledHandler := handlers.NewDisabledHandler()
	return newWrapper(disabledHandler)
}
