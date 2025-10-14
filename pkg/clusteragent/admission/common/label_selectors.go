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

// DefaultLabelSelectors returns the mutating webhooks object selector based on the configuration
func DefaultLabelSelectors(useNamespaceSelector bool, nsLabelOpt ...func(*metav1.LabelSelector)) (namespaceSelector, objectSelector *metav1.LabelSelector) {
	var nsLabelSelector, objLabelSelector *metav1.LabelSelector

	if pkgconfigsetup.Datadog().GetBool("admission_controller.mutate_unlabelled") ||
		pkgconfigsetup.Datadog().GetBool("apm_config.instrumentation.enabled") ||
		len(pkgconfigsetup.Datadog().GetStringSlice("apm_config.instrumentation.enabled_namespaces")) > 0 {
		// Accept all, ignore pods if they're explicitly filtered-out
		nsLabelSelector = &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      EnabledLabelKey,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"false"},
				},
			},
		}
	} else {
		// Ignore all, accept pods if they're explicitly allowed
		objLabelSelector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				EnabledLabelKey: "true",
			},
		}
	}

	if len(nsLabelOpt) > 0 {
		if nsLabelSelector == nil {
			nsLabelSelector = &metav1.LabelSelector{}
		}

		for _, opt := range nsLabelOpt {
			opt(nsLabelSelector)
		}
	}

	if pkgconfigsetup.Datadog().GetBool("admission_controller.add_aks_selectors") {
		nsLabelSelector, objLabelSelector = aksSelectors(useNamespaceSelector, nsLabelSelector, objLabelSelector)
	}

	if useNamespaceSelector {
		return nsLabelSelector, nil
	}

	return nsLabelSelector, objLabelSelector
}

// ExcludeNamespaces returns a function that excludes the given namespaces from the provided label selector
func ExcludeNamespaces(excludedNs []string) func(*metav1.LabelSelector) {
	return func(nsLabel *metav1.LabelSelector) {
		nsLabel.MatchExpressions = append(nsLabel.MatchExpressions, metav1.LabelSelectorRequirement{
			Key:      KubeSystemNamespaceLabelKey,
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   excludedNs,
		})
	}
}

// aksSelectors takes a label selector and builds a namespace and object
// selector adapted for AKS. AKS adds automatically some selector requirements
// if we don't, so we need to add them to avoid conflicts when updating the
// webhook.
//
// Ref: https://docs.microsoft.com/en-us/azure/aks/faq#can-i-use-admission-controller-webhooks-on-aks
// Ref: https://github.com/Azure/AKS/issues/1771
func aksSelectors(useNamespaceSelector bool, nsLabelSelector, objSelector *metav1.LabelSelector) (*metav1.LabelSelector, *metav1.LabelSelector) {
	if useNamespaceSelector {
		if nsLabelSelector == nil {
			nsLabelSelector = &metav1.LabelSelector{}
		}
		nsLabelSelector.MatchExpressions = append(
			nsLabelSelector.MatchExpressions,
			azureAKSLabelSelectorRequirement()...,
		)
		return nsLabelSelector, nil
	}

	// Azure AKS adds the namespace selector even in Kubernetes versions that
	// support object selectors, so we need to add it to avoid conflicts.
	if objSelector == nil {
		objSelector = &metav1.LabelSelector{}
	}
	objSelector.MatchExpressions = append(
		objSelector.MatchExpressions,
		azureAKSLabelSelectorRequirement()...,
	)
	return nsLabelSelector, objSelector
}

func azureAKSLabelSelectorRequirement() []metav1.LabelSelectorRequirement {
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
