// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
)

// ScrubMutatingWebhookConfiguration scrubs the metadata of a MutatingWebhookConfiguration.
func ScrubMutatingWebhookConfiguration(r *admissionregistrationv1.MutatingWebhookConfiguration, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubValidatingAdmissionPolicy scrubs the metadata of a ValidatingAdmissionPolicy.
func ScrubValidatingAdmissionPolicy(r *admissionregistrationv1.ValidatingAdmissionPolicy, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubValidatingAdmissionPolicyBinding scrubs the metadata of a ValidatingAdmissionPolicyBinding.
func ScrubValidatingAdmissionPolicyBinding(r *admissionregistrationv1.ValidatingAdmissionPolicyBinding, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubValidatingWebhookConfiguration scrubs the metadata of a ValidatingWebhookConfiguration.
func ScrubValidatingWebhookConfiguration(r *admissionregistrationv1.ValidatingWebhookConfiguration, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}
