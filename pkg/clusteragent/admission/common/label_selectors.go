// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// LabelSelectorsConfig configures the namespace label selectors for admission webhooks
type LabelSelectorsConfig struct {
	// ExcludeNamespaces lists namespaces to exclude from webhook invocation
	ExcludeNamespaces []string

	// IncludeNamespaces lists namespaces to explicitly include (mutually exclusive with ExcludeNamespaces)
	// If set, only these namespaces will be targeted
	IncludeNamespaces []string
}

// DefaultLabelSelectors returns namespace and object selectors for admission webhooks.
//
// If useNamespaceSelector is true, only a namespace selector will be used for matching. This is necessary
// for older Kubernetes versions that do not support object selectors.
func DefaultLabelSelectors(useNamespaceSelector bool, config LabelSelectorsConfig) (nsSelector *metav1.LabelSelector, objSelector *metav1.LabelSelector) {
	nsSelector = &metav1.LabelSelector{}
	if useNamespaceSelector {
		applyAdmissionEnabledSelectors(nsSelector)
	} else {
		objSelector = &metav1.LabelSelector{}
		applyAdmissionEnabledSelectors(objSelector)
	}

	applySelectorConfig(nsSelector, config)

	if pkgconfigsetup.Datadog().GetBool("admission_controller.add_aks_selectors") {
		// AKS automatically adds some selector requirements if we don't
		// so we need to add them to avoid conflicts when updating the webhook.
		//
		// Ref: https://docs.microsoft.com/en-us/azure/aks/faq#can-i-use-admission-controller-webhooks-on-aks
		nsSelector.MatchExpressions = append(
			nsSelector.MatchExpressions,
			AzureAKSLabelSelectorRequirement()...,
		)
	}
	return nsSelector, objSelector
}

func applySelectorConfig(nsSelector *metav1.LabelSelector, config LabelSelectorsConfig) {
	if len(config.IncludeNamespaces) > 0 {
		nsSelector.MatchExpressions = append(nsSelector.MatchExpressions, metav1.LabelSelectorRequirement{
			Key:      NamespaceLabelKey,
			Operator: metav1.LabelSelectorOpIn,
			Values:   config.IncludeNamespaces,
		})
	} else if len(config.ExcludeNamespaces) > 0 {
		nsSelector.MatchExpressions = append(nsSelector.MatchExpressions, metav1.LabelSelectorRequirement{
			Key:      NamespaceLabelKey,
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   config.ExcludeNamespaces,
		})
	}
}

func applyAdmissionEnabledSelectors(selector *metav1.LabelSelector) {
	if pkgconfigsetup.Datadog().GetBool("admission_controller.mutate_unlabelled") {
		// Accept all, ignore pods explicitly filtered-out
		selector.MatchExpressions = []metav1.LabelSelectorRequirement{
			{
				Key:      EnabledLabelKey,
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"false"},
			},
		}
	} else {
		// Ignore all, accept pods explicitly allowed
		selector.MatchLabels = map[string]string{
			EnabledLabelKey: "true",
		}
	}
}

func AzureAKSLabelSelectorRequirement() []metav1.LabelSelectorRequirement {
	return []metav1.LabelSelectorRequirement{
		{
			Key:      "control-plane",
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      "control-plane",
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   []string{"true"},
		},
		{
			Key:      "kubernetes.azure.com/managedby",
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   []string{"aks"},
		},
	}
}
