// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

// Package handlermock provides a configurable instrumentation handler for component tests.
package handlermock

import (
	"context"
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

// Handler is a test double. Set callbacks before using it concurrently.
type Handler struct {
	HandlerName        string
	HasSectionFunc     func(*datadoghq.DatadogInstrumentation) bool
	SupportsTargetFunc func(autoscalingv2.CrossVersionObjectReference) bool
	HandleFunc         func(context.Context, instrumentation.EventType, *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error)
	ValidateFunc       func(*datadoghq.DatadogInstrumentation) []instrumentation.ValidationError
}

// New creates a handler whose default methods have no side effects.
func New(t *testing.T, name string) *Handler {
	t.Helper()
	return &Handler{HandlerName: name}
}

// Name returns the configured handler name.
func (h *Handler) Name() string { return h.HandlerName }

// HasSection delegates to HasSectionFunc, defaulting to false.
func (h *Handler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return h.HasSectionFunc != nil && h.HasSectionFunc(cr)
}

// SupportsTarget delegates to SupportsTargetFunc, defaulting to false.
func (h *Handler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	return h.SupportsTargetFunc != nil && h.SupportsTargetFunc(ref)
}

// Handle delegates to HandleFunc, defaulting to an empty status.
func (h *Handler) Handle(ctx context.Context, event instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
	if h.HandleFunc != nil {
		return h.HandleFunc(ctx, event, cr)
	}
	return instrumentation.HandlerStatus{}, nil
}

// Validate delegates to ValidateFunc, defaulting to no errors.
func (h *Handler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	if h.ValidateFunc != nil {
		return h.ValidateFunc(cr)
	}
	return nil
}
