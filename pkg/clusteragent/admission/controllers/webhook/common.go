// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package webhook

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// buildLabelSelector returns the mutating webhooks object selector based on the configuration
func buildLabelSelector() *metav1.LabelSelector {
	if config.Datadog.GetBool("admission_controller.mutate_unlabelled") {
		// Accept all, ignore pods if they're explicitly filtered-out
		return &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      common.EnabledLabelKey,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"false"},
				},
			},
		}
	}

	// Ignore all, accept pods if they're explicitly allowed
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			common.EnabledLabelKey: "true",
		},
	}
}
