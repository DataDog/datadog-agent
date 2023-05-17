// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package webhook

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// buildLabelSelectors returns the mutating webhooks object selector based on the configuration
func buildLabelSelectors(useNamespaceSelector bool) (namespaceSelector, objectSelector *metav1.LabelSelector) {
	var labelSelector metav1.LabelSelector

	if config.Datadog.GetBool("admission_controller.mutate_unlabelled") {
		// Accept all, ignore pods if they're explicitly filtered-out
		labelSelector = metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      common.EnabledLabelKey,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"false"},
				},
			},
		}
	} else {
		// Ignore all, accept pods if they're explicitly allowed
		labelSelector = metav1.LabelSelector{
			MatchLabels: map[string]string{
				common.EnabledLabelKey: "true",
			},
		}
	}

	if config.Datadog.GetBool("admission_controller.add_aks_selectors") {
		return aksSelectors(useNamespaceSelector, labelSelector)
	}

	if useNamespaceSelector {
		return &labelSelector, nil
	}

	return nil, &labelSelector
}

// aksSelectors takes a label selector and builds a namespace and object
// selector adapted for AKS. AKS adds automatically some selector requirements
// if we don't, so we need to add them to avoid conflicts when updating the
// webhook.
//
// Ref: https://docs.microsoft.com/en-us/azure/aks/faq#can-i-use-admission-controller-webhooks-on-aks
// Ref: https://github.com/Azure/AKS/issues/1771
func aksSelectors(useNamespaceSelector bool, labelSelector metav1.LabelSelector) (namespaceSelector, objectSelector *metav1.LabelSelector) {
	if useNamespaceSelector {
		labelSelector.MatchExpressions = append(
			labelSelector.MatchExpressions,
			azureAKSLabelSelectorRequirement(),
		)
		return &labelSelector, nil
	}

	// Azure AKS adds the namespace selector even in Kubernetes versions that
	// support object selectors, so we need to add it to avoid conflicts.
	return &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			azureAKSLabelSelectorRequirement(),
		},
	}, &labelSelector
}

func azureAKSLabelSelectorRequirement() metav1.LabelSelectorRequirement {
	return metav1.LabelSelectorRequirement{
		Key:      "control-plane",
		Operator: metav1.LabelSelectorOpDoesNotExist,
	}
}
