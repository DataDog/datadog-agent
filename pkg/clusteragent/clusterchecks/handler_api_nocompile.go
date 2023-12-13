// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !clusterchecks

package clusterchecks

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

var (
	// ErrNotCompiled is returned if cluster check support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("cluster-check support not compiled in")
)

// Handler not implemented
type Handler struct{}

// GetState not implemented
func (h *Handler) GetState() (types.StateResponse, error) {
	return types.StateResponse{}, ErrNotCompiled
}

// NewHandler not implemented
func NewHandler(_ *autodiscovery.AutoConfig) (*Handler, error) {
	return nil, ErrNotCompiled
}

// Run not implemented
func (h *Handler) Run(_ context.Context) error {
	return ErrNotCompiled
}

// GetStats not implemented
func GetStats() (*types.Stats, error) {
	return nil, ErrNotCompiled
}
