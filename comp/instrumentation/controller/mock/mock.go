// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

// Package mock provides a test double for the instrumentation controller.
package mock

import (
	"context"
	"testing"

	controller "github.com/DataDog/datadog-agent/comp/instrumentation/controller/def"
)

// Controller is a test double. Set StartFunc before calling Start.
type Controller struct {
	StartFunc func(context.Context, controller.Params) error
}

var _ controller.Component = (*Controller)(nil)

// Mock creates a controller that does nothing unless StartFunc is set.
func Mock(t *testing.T) *Controller { t.Helper(); return &Controller{} }

// Start delegates to StartFunc when configured.
func (c *Controller) Start(ctx context.Context, params controller.Params) error {
	if c.StartFunc != nil {
		return c.StartFunc(ctx, params)
	}
	return nil
}
