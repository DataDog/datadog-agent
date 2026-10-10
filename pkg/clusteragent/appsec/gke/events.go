// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package gke

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

// Event reasons for GCPTrafficExtension operations
const (
	EventReasonGCPTrafficExtensionCreated      = "GCPTrafficExtensionCreated"
	EventReasonGCPTrafficExtensionCreateFailed = "GCPTrafficExtensionCreateFailed"
	EventReasonGCPTrafficExtensionDeleted      = "GCPTrafficExtensionDeleted"
	EventReasonGCPTrafficExtensionDeleteFailed = "GCPTrafficExtensionDeleteFailed"
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

// GCPTrafficExtension event recording methods (recorded on Gateway)

func (e *eventRecorder) recordExtensionCreated(gatewayNamespace, gatewayName, extensionName string) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeNormal,
		EventReasonGCPTrafficExtensionCreated,
		"Created GCPTrafficExtension %q to enable AppSec processing",
		extensionName,
	)
}

func (e *eventRecorder) recordExtensionCreateFailed(gatewayNamespace, gatewayName, extensionName string, err error) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeWarning,
		EventReasonGCPTrafficExtensionCreateFailed,
		"Failed to create GCPTrafficExtension %q: %v",
		extensionName,
		err,
	)
}

func (e *eventRecorder) recordExtensionDeleted(gatewayNamespace, gatewayName, extensionName string) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeNormal,
		EventReasonGCPTrafficExtensionDeleted,
		"Deleted GCPTrafficExtension %q",
		extensionName,
	)
}

func (e *eventRecorder) recordExtensionDeleteFailed(gatewayNamespace, gatewayName, extensionName string, err error) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeWarning,
		EventReasonGCPTrafficExtensionDeleteFailed,
		"Failed to delete GCPTrafficExtension %q: %v",
		extensionName,
		err,
	)
}
