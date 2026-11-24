// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testing

import (
	"testing"

	admission "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/admission"
)

func TestGeneratedAdmissionActionsRejectMissingInputs(t *testing.T) {
	assertGeneratedActionsRejectMissingInputs(t, admission.NewKubernetesAdmissionregistration(), []generatedActionResource{
		{name: "MutatingWebhookConfiguration"},
		{name: "ValidatingWebhookConfiguration"},
		{name: "ValidatingAdmissionPolicy"},
		{name: "ValidatingAdmissionPolicyBinding"},
	})
}
