// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !clusterchecks

package clusterchecks

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

var (
	// ErrNotCompiled is returned if cluster check support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("cluster-check support not compiled in")
)

// Handler not implemented
type Handler struct{}

// GetAllConfigs not implemented
func (h *Handler) GetAllConfigs() ([]integration.Config, error) {
	return nil, ErrNotCompiled
}

// NewHandler not implemented
func NewHandler(_ *autodiscovery.AutoConfig) (*Handler, error) {
	return nil, ErrNotCompiled
}

// Run not implemented
func (h *Handler) Run(_ context.Context) error {
	return ErrNotCompiled
}
