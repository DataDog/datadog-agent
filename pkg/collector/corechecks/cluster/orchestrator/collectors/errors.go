// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package collectors

import (
	"fmt"

	"github.com/pkg/errors"
)

var (
	// ErrProcessingPanic is the error raised when a panic was caught on resource
	// processing.
	ErrProcessingPanic = fmt.Errorf("unable to process resources: a panic occurred")
)

// NewListingError creates an error that wraps the cause of a listing failure.
func NewListingError(cause error) error {
	return errors.WithMessage(cause, "unable to list resources")
}

// NewProcessingError creates an error that wraps the cause of a processing
// failure.
func NewProcessingError(cause error) error {
	return errors.WithMessage(cause, "unable to process resources")
}
