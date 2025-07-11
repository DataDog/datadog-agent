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

	// LibVersionAnnotKeyFormat is the format of the library version annotation
	LibVersionAnnotKeyFormat = "admission.datadoghq.com/%s-lib.version"

	// LibConfigV1AnnotKeyFormat is the format of the library config annotation
	LibConfigV1AnnotKeyFormat = "admission.datadoghq.com/%s-lib.config.v1"
)
