// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package envoygateway

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

// Event reasons for EnvoyPatchPolicy operations
const (
	EventReasonPatchPolicyCreated      = "EnvoyPatchPolicyCreated"
	EventReasonPatchPolicyCreateFailed = "EnvoyPatchPolicyCreateFailed"
	EventReasonPatchPolicyDeleted      = "EnvoyPatchPolicyDeleted"
	EventReasonPatchPolicyDeleteFailed = "EnvoyPatchPolicyDeleteFailed"
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
		APIVersion: "gateway.networking.k8s.io/v1",
	}
}

// EnvoyPatchPolicy event recording methods (recorded on Gateway)

func (e *eventRecorder) recordPatchPolicyCreated(gatewayNamespace, gatewayName string) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeNormal,
		EventReasonPatchPolicyCreated,
		"Created EnvoyPatchPolicy %q to enable AppSec processing",
		patchPolicyName(gatewayName),
	)
}

func (e *eventRecorder) recordPatchPolicyCreateFailed(gatewayNamespace, gatewayName string, err error) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeWarning,
		EventReasonPatchPolicyCreateFailed,
		"Failed to create EnvoyPatchPolicy %q: %v",
		patchPolicyName(gatewayName),
		err,
	)
}

func (e *eventRecorder) recordPatchPolicyDeleted(gatewayNamespace, gatewayName string) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeNormal,
		EventReasonPatchPolicyDeleted,
		"Deleted EnvoyPatchPolicy %q",
		patchPolicyName(gatewayName),
	)
}

func (e *eventRecorder) recordPatchPolicyDeleteFailed(gatewayNamespace, gatewayName string, err error) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeWarning,
		EventReasonPatchPolicyDeleteFailed,
		"Failed to delete EnvoyPatchPolicy %q: %v",
		patchPolicyName(gatewayName),
		err,
	)
}
