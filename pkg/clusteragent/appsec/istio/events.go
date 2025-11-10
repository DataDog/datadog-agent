// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package istio

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

// Event reasons for EnvoyFilter operations
const (
	EventReasonExtensionPolicyCreated      = "EnvoyFilterCreated"
	EventReasonExtensionPolicyCreateFailed = "EnvoyFilterCreateFailed"
	EventReasonExtensionPolicyDeleted      = "EnvoyFilterDeleted"
	EventReasonExtensionPolicyDeleteFailed = "EnvoyFilterDeleteFailed"
)

// eventRecorder provides methods to record Kubernetes events for appsec resources
type eventRecorder struct {
	recorder record.EventRecorder
}

// getGatewayReference returns an ObjectReference for a Gateway
func (e *eventRecorder) getGatewayReference(namespace, name string) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		Kind:       "Gateway",
		Namespace:  namespace,
		Name:       name,
		APIVersion: "gateway.networking.istio.io/v1",
	}
}

func (e *eventRecorder) recordExtensionPolicyCreated(gatewayNamespace, gatewayName string) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeNormal,
		EventReasonExtensionPolicyCreated,
		"Created EnvoyFilter %q to enable Appsec processing",
		envoyFilterName,
	)
}

func (e *eventRecorder) recordExtensionPolicyCreateFailed(gatewayNamespace, gatewayName string, err error) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeWarning,
		EventReasonExtensionPolicyCreateFailed,
		"Failed to create EnvoyFilter %q: %v",
		envoyFilterName,
		err,
	)
}

func (e *eventRecorder) recordExtensionPolicyDeleted(gatewayNamespace, gatewayName string) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeNormal,
		EventReasonExtensionPolicyDeleted,
		"Deleted EnvoyFilter %q",
		envoyFilterName,
	)
}

func (e *eventRecorder) recordExtensionPolicyDeleteFailed(gatewayNamespace, gatewayName string, err error) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeWarning,
		EventReasonExtensionPolicyDeleteFailed,
		"Failed to delete EnvoyFilter %q: %v",
		envoyFilterName,
		err,
	)
}
