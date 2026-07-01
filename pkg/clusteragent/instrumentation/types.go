// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package instrumentation reconciles DatadogInstrumentation custom resources.
package instrumentation

import (
	"context"
	"fmt"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var DatadogInstrumentationGVR = datadoghq.GroupVersion.WithResource("datadoginstrumentations")

// EventType describes the section-level event dispatched to product handlers.
type EventType string

const (
	// EventCreate is emitted when a handler section is newly present.
	EventCreate EventType = "create"
	// EventUpdate is emitted when a handler section remains present across an update.
	EventUpdate EventType = "update"
	// EventDelete is emitted when a handler section is removed or the CR is deleted.
	EventDelete EventType = "delete"
)

// Handler owns one product section of a DatadogInstrumentation custom resource.
type Handler interface {
	// Name returns the unique handler name.
	Name() string
	// HasSection returns true if the CR has a config section relevant to this handler.
	HasSection(*datadoghq.DatadogInstrumentation) bool
	// SupportsTarget returns true if this handler supports the given target kind.
	SupportsTarget(autoscalingv2.CrossVersionObjectReference) bool
	// Handle is called when a relevant CR event occurs for this handler's config section. Handlers also must
	// be idempotent, as the controller may send duplicate events for the same change.
	//
	// Events are dispatched to handlers regardless of the cluster agent's leadership/follower state. If leadership
	// matters, then the handler should determine if it's the leader before acting.
	Handle(context.Context, EventType, *datadoghq.DatadogInstrumentation) (HandlerStatus, error)
	// Validate checks product-specific fields during webhook admission.
	Validate(*datadoghq.DatadogInstrumentation) []ValidationError
}

// HandlerStatus is the controller-facing status contract returned by handlers.
type HandlerStatus struct {
	Type    string
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// ValidationError describes a reusable validation failure for reconciliation status
// and future admission-webhook rejection messages.
type ValidationError struct {
	Type        string
	Reason      string
	Message     string
	Field       string
	HandlerName string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e ValidationError) HandlerStatus() HandlerStatus {
	return HandlerStatus{
		Type:    e.Type,
		Status:  metav1.ConditionFalse,
		Reason:  e.Reason,
		Message: e.Message,
	}
}
