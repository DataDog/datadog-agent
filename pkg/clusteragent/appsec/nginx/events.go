// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package nginx

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

// Event reasons for ingress-nginx ConfigMap operations
const (
	EventReasonConfigMapCreated      = "DatadogConfigMapCreated"
	EventReasonConfigMapCreateFailed = "DatadogConfigMapCreateFailed"
	EventReasonConfigMapDeleted      = "DatadogConfigMapDeleted"
	EventReasonConfigMapDeleteFailed = "DatadogConfigMapDeleteFailed"
	EventReasonVersionParseFailed    = "VersionParseFailed"
)

// eventRecorder provides methods to record Kubernetes events for appsec nginx resources
type eventRecorder struct {
	recorder record.EventRecorder
}

func (e *eventRecorder) ingressClassRef(name string) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		Kind:       "IngressClass",
		Name:       name,
		APIVersion: "networking.k8s.io/v1",
	}
}

func (e *eventRecorder) recordConfigMapCreated(ingressClassName, ddConfigMapName string) {
	e.recorder.Eventf(
		e.ingressClassRef(ingressClassName),
		corev1.EventTypeNormal,
		EventReasonConfigMapCreated,
		"Created Datadog AppSec ConfigMap %q for ingress-nginx",
		ddConfigMapName,
	)
}

func (e *eventRecorder) recordConfigMapCreateFailed(ingressClassName string, err error) {
	e.recorder.Eventf(
		e.ingressClassRef(ingressClassName),
		corev1.EventTypeWarning,
		EventReasonConfigMapCreateFailed,
		"Failed to create Datadog AppSec ConfigMap: %v",
		err,
	)
}

func (e *eventRecorder) recordConfigMapDeleted(ingressClassName string) {
	e.recorder.Eventf(
		e.ingressClassRef(ingressClassName),
		corev1.EventTypeNormal,
		EventReasonConfigMapDeleted,
		"Deleted Datadog AppSec ConfigMap for ingress-nginx",
	)
}

func (e *eventRecorder) recordConfigMapDeleteFailed(ingressClassName string, err error) {
	e.recorder.Eventf(
		e.ingressClassRef(ingressClassName),
		corev1.EventTypeWarning,
		EventReasonConfigMapDeleteFailed,
		"Failed to delete Datadog AppSec ConfigMap: %v",
		err,
	)
}

func (e *eventRecorder) recordVersionParseFailed(podName, image string) {
	e.recorder.Eventf(
		&corev1.ObjectReference{
			Kind:       "Pod",
			Name:       podName,
			APIVersion: "v1",
		},
		corev1.EventTypeWarning,
		EventReasonVersionParseFailed,
		"Failed to parse ingress-nginx version from image %q. Follow the manual extraModules process to enable AppSec.",
		image,
	)
}
