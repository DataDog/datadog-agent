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

// Event reasons for ReferenceGrant operations
const (
	EventReasonReferenceGrantCreated      = "ReferenceGrantCreated"
	EventReasonReferenceGrantCreateFailed = "ReferenceGrantCreateFailed"
	EventReasonReferenceGrantUpdateFailed = "ReferenceGrantUpdateFailed"
	EventReasonReferenceGrantDeleteFailed = "ReferenceGrantDeleteFailed"
	EventReasonNamespaceAdded             = "ReferenceGrantNamespaceAdded"
	EventReasonNamespaceRemoved           = "ReferenceGrantNamespaceRemoved"
)

// Event reasons for EnvoyExtensionPolicy operations
const (
	EventReasonExtensionPolicyCreated      = "EnvoyExtensionPolicyCreated"
	EventReasonExtensionPolicyCreateFailed = "EnvoyExtensionPolicyCreateFailed"
	EventReasonExtensionPolicyDeleted      = "EnvoyExtensionPolicyDeleted"
	EventReasonExtensionPolicyDeleteFailed = "EnvoyExtensionPolicyDeleteFailed"
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

// getExtensionPolicyReference returns an ObjectReference for an EnvoyExtensionPolicy
func (e *eventRecorder) getExtensionPolicyReference(namespace string) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		Kind:       "EnvoyExtensionPolicy",
		Namespace:  namespace,
		Name:       extProcName,
		APIVersion: "gateway.envoyproxy.io/v1alpha1",
	}
}

// ReferenceGrant event recording methods (recorded on EnvoyExtensionPolicy)

func (e *eventRecorder) recordReferenceGrantCreated(policyNamespace string, grantNamespace string) {
	e.recorder.Eventf(
		e.getExtensionPolicyReference(policyNamespace),
		corev1.EventTypeNormal,
		EventReasonReferenceGrantCreated,
		"Created ReferenceGrant %q to allow cross-namespace access from namespace %s",
		referenceGrantName,
		grantNamespace,
	)
}

func (e *eventRecorder) recordReferenceGrantCreateFailed(policyNamespace string, grantNamespace string, err error) {
	e.recorder.Eventf(
		e.getExtensionPolicyReference(policyNamespace),
		corev1.EventTypeWarning,
		EventReasonReferenceGrantCreateFailed,
		"Failed to create ReferenceGrant %q for namespace %q: %v",
		referenceGrantName,
		grantNamespace,
		err,
	)
}

func (e *eventRecorder) recordNamespaceAddedToGrant(policyNamespace string, grantNamespace string) {
	e.recorder.Eventf(
		e.getExtensionPolicyReference(policyNamespace),
		corev1.EventTypeNormal,
		EventReasonNamespaceAdded,
		"Added namespace %q to ReferenceGrant %q for cross-namespace access",
		grantNamespace,
		referenceGrantName,
	)
}

func (e *eventRecorder) recordNamespaceAddFailed(policyNamespace string, grantNamespace string, err error) {
	e.recorder.Eventf(
		e.getExtensionPolicyReference(policyNamespace),
		corev1.EventTypeWarning,
		EventReasonReferenceGrantUpdateFailed,
		"Failed to add namespace %q to ReferenceGrant: %v",
		grantNamespace,
		err,
	)
}

func (e *eventRecorder) recordNamespaceRemovedFromGrant(policyNamespace string, grantNamespace string) {
	e.recorder.Eventf(
		e.getExtensionPolicyReference(policyNamespace),
		corev1.EventTypeNormal,
		EventReasonNamespaceRemoved,
		"Removed namespace %q from ReferenceGrant %q",
		grantNamespace,
		referenceGrantName,
	)
}

func (e *eventRecorder) recordNamespaceRemovalFailed(policyNamespace string, grantNamespace string, err error) {
	e.recorder.Eventf(
		e.getExtensionPolicyReference(policyNamespace),
		corev1.EventTypeWarning,
		EventReasonReferenceGrantUpdateFailed,
		"Failed to remove namespace %q from ReferenceGrant %q: %v",
		grantNamespace,
		referenceGrantName,
		err,
	)
}

func (e *eventRecorder) recordReferenceGrantDeleteFailed(policyNamespace string, err error) {
	e.recorder.Eventf(
		e.getExtensionPolicyReference(policyNamespace),
		corev1.EventTypeWarning,
		EventReasonReferenceGrantDeleteFailed,
		"Failed to delete ReferenceGrant: %v",
		err,
	)
}

// EnvoyExtensionPolicy event recording methods (recorded on Gateway)

func (e *eventRecorder) recordExtensionPolicyCreated(gatewayNamespace, gatewayName string) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeNormal,
		EventReasonExtensionPolicyCreated,
		"Created EnvoyExtensionPolicy %q to enable AppSec processing",
		extProcName,
	)
}

func (e *eventRecorder) recordExtensionPolicyCreateFailed(gatewayNamespace, gatewayName string, err error) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeWarning,
		EventReasonExtensionPolicyCreateFailed,
		"Failed to create EnvoyExtensionPolicy %q: %v",
		extProcName,
		err,
	)
}

func (e *eventRecorder) recordExtensionPolicyDeleted(gatewayNamespace, gatewayName string) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeNormal,
		EventReasonExtensionPolicyDeleted,
		"Deleted EnvoyExtensionPolicy %q",
		extProcName,
	)
}

func (e *eventRecorder) recordExtensionPolicyDeleteFailed(gatewayNamespace, gatewayName string, err error) {
	e.recorder.Eventf(
		e.getGatewayReference(gatewayNamespace, gatewayName),
		corev1.EventTypeWarning,
		EventReasonExtensionPolicyDeleteFailed,
		"Failed to delete EnvoyExtensionPolicy %q: %v",
		extProcName,
		err,
	)
}
