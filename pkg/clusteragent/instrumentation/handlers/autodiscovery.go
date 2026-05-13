// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const checksReadyConditionType = "ChecksReady"

// TODO: Implement check translation and delivery.

// AutodiscoveryHandler is the shell for DatadogInstrumentation check configuration handling.
type AutodiscoveryHandler struct{}

// NewAutodiscoveryHandler returns the Autodiscovery DatadogInstrumentation handler.
func NewAutodiscoveryHandler(_ Deps) *AutodiscoveryHandler {
	return &AutodiscoveryHandler{}
}

// Name returns the unique handler name.
func (h *AutodiscoveryHandler) Name() string {
	return "autodiscovery"
}

// HasSection reports whether the CR contains Autodiscovery check configuration.
func (h *AutodiscoveryHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && len(cr.Spec.Config.Checks) > 0
}

// SupportsTarget returns whether Autodiscovery check delivery supports the target kind.
func (h *AutodiscoveryHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	switch ref.Kind {
	case "Deployment", "DaemonSet", "StatefulSet", "CronJob", "Job", "Service":
		return true
	default:
		return false
	}
}

// Validate performs no additional validation beyond CRD schema validation for now.
func (h *AutodiscoveryHandler) Validate(_ *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	return nil
}

// Handle is intentionally non-functional until the Autodiscovery delivery implementation is added.
func (h *AutodiscoveryHandler) Handle(_ context.Context, _ instrumentation.EventType, _ *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
	return instrumentation.HandlerStatus{
		Type:    checksReadyConditionType,
		Status:  metav1.ConditionUnknown,
		Reason:  "HandlerNotImplemented",
		Message: "Autodiscovery DatadogInstrumentation handling is not implemented yet.",
	}, nil
}
