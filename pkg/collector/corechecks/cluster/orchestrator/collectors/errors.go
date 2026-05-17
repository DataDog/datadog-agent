// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package collectors

import (
	"errors"
	"fmt"
)

var (
	// ErrProcessingPanic is the error raised when a panic was caught on resource
	// processing.
	ErrProcessingPanic = errors.New("unable to process resources: a panic occurred")
)

// NewListingError creates an error that wraps the cause of a listing failure.
func NewListingError(cause error) error {
	return fmt.Errorf("unable to list resources: %w", cause)
}

// NewProcessingError creates an error that wraps the cause of a processing
// failure.
func NewProcessingError(cause error) error {
	return fmt.Errorf("unable to process resources: %w", cause)
}
