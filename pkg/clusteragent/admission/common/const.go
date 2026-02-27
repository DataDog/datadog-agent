// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package common defines constants and types used by the Admission Controller.
package common

// WebhookType is the type of the webhook.
type WebhookType string

// String returns the string representation of the WebhookType.
func (t WebhookType) String() string {
	return string(t)
}

const (
	// ValidatingWebhook is type for Validating Webhooks.
	ValidatingWebhook = "validating"
	// MutatingWebhook is type for Mutating Webhooks.
	MutatingWebhook = "mutating"
)

const (
	// EnabledLabelKey pod label to disable/enable mutations at the pod level.
	EnabledLabelKey = "admission.datadoghq.com/enabled"

	// InjectionModeLabelKey pod label to choose the config injection at the pod level.
	InjectionModeLabelKey = "admission.datadoghq.com/config.mode"

	// TypeSocketVolumesLabelKey pod label to decide if socket volume type should be used.
	TypeSocketVolumesLabelKey = "admission.datadoghq.com/config.type_socket_volumes"

	// NamespaceLabelKey label to select resources based on namespace.
	// This label was added in Kubernetes 1.22, and won't work on older k8s versions.
	// See https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/#automatic-labelling
	NamespaceLabelKey = "kubernetes.io/metadata.name"

	// MutatedByWebhookAnnotationKey is set on pods that have been mutated by the admission webhook.
	// Its presence indicates the webhook was successfully invoked for the pod.
	MutatedByWebhookAnnotationKey = "admission.datadoghq.com/mutated"
)
