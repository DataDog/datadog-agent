// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"context"
	"errors"
	"fmt"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
)

// CreateTimeoutContext creates a context with timeout if timeoutSeconds is provided and greater than 0.
// Otherwise, it returns the original context with a no-op cancel function.
func CreateTimeoutContext(ctx context.Context, timeoutSeconds *int32) (context.Context, context.CancelFunc) {
	if timeoutSeconds != nil && *timeoutSeconds > 0 {
		timeout := time.Duration(*timeoutSeconds) * time.Second
		return context.WithTimeout(ctx, timeout)
	}
	return ctx, func() {}
}

// HandleTimeoutError checks if an error occurred due to context deadline exceeded,
// logs a warning if so, and returns whether a timeout occurred along with a formatted error message.
// This is used by both AppBuilderRunner and WorkflowRunner to handle task timeouts consistently.
func HandleTimeoutError(ctx context.Context, err error, timeoutSeconds *int32, logger log.Logger) (bool, error) {
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		logger.Warn("task execution timed out due to global timeout",
			log.Int32("timeout_seconds", *timeoutSeconds))
		return true, fmt.Errorf("task execution exceeded global timeout of %d seconds", *timeoutSeconds)
	}
	return false, nil
}
