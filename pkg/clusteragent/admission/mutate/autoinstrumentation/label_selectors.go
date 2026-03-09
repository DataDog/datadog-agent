// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

// LabrlSelectorsConfig provides configuration values for NewLabelSelectors.
type LabelSelectorsConfig struct {
	Enabled            bool
	MutateUnlabelled   bool
	AddAksSelectors    bool
	EnabledNamespaces              []string
	EnabledNamespacesWebhookFilter bool
	DisabledNamespaces             []string
}

// NewLabelSelectorsConfig initializes a config object from the datadog config.
func NewLabelSelectorsConfig(datadogConfig config.Component) *LabelSelectorsConfig {
	return &LabelSelectorsConfig{
		Enabled:            datadogConfig.GetBool("apm_config.instrumentation.enabled"),
		MutateUnlabelled:   datadogConfig.GetBool("admission_controller.mutate_unlabelled"),
		AddAksSelectors:    datadogConfig.GetBool("admission_controller.add_aks_selectors"),
		EnabledNamespaces:              datadogConfig.GetStringSlice("apm_config.instrumentation.enabled_namespaces"),
		EnabledNamespacesWebhookFilter: datadogConfig.GetBool("admission_controller.auto_instrumentation.webhook_filter_namespaces"),
		DisabledNamespaces:             datadogConfig.GetStringSlice("apm_config.instrumentation.disabled_namespaces"),
	}
}

// LabelSelectors is a helper object that provides a namespaceSelector and objectSelector used by the webhook.
type LabelSelectors struct {
	config *LabelSelectorsConfig
}

// NewLabelSelectors initializes a LabelSelectors with the provided config.
func NewLabelSelectors(config *LabelSelectorsConfig) *LabelSelectors {
	return &LabelSelectors{
		config: config,
	}
}

// Get returns the namespace and object selectors based on the configuration and the optional useNamespaceSelector.
func (ls *LabelSelectors) Get(useNamespaceSelector bool) (*metav1.LabelSelector, *metav1.LabelSelector) {
	var objectSelector *metav1.LabelSelector

	// useNamespaceSelector determines whether we need to fallback to using only the namespace selector instead of the
	// combination with the object selector. This is to support k8s version is between 1.10 and 1.14 (included).
	// Kubernetes 1.15+ supports object selectors.
	namespaceSelector := &metav1.LabelSelector{}
	if useNamespaceSelector {
		ls.setupObjectSelector(namespaceSelector)
	} else {
		objectSelector = &metav1.LabelSelector{}
		ls.setupObjectSelector(objectSelector)
	}

	// Setup disabled namespaces.
	disabledNamespaces := mutatecommon.DefaultDisabledNamespaces()
	disabledNamespaces = append(disabledNamespaces, ls.config.DisabledNamespaces...)

	// Apply disabled namespaces so we don't even receive mutation requests for them.
	namespaceSelector.MatchExpressions = append(namespaceSelector.MatchExpressions, metav1.LabelSelectorRequirement{
		Key:      common.NamespaceLabelKey,
		Operator: metav1.LabelSelectorOpNotIn,
		Values:   disabledNamespaces,
	})

	// Apply enabled namespaces so we only receive mutation requests for them.
	// This is gated behind a config flag because it changes behavior for pods that use per-pod opt-in
	// (admission.datadoghq.com/enabled=true) outside of enabled namespaces. Without this flag, those pods
	// are still mutated at the application level; with this flag, the API server drops the request before
	// it reaches the webhook.
	if ls.config.EnabledNamespacesWebhookFilter && len(ls.config.EnabledNamespaces) > 0 {
		namespaceSelector.MatchExpressions = append(namespaceSelector.MatchExpressions, metav1.LabelSelectorRequirement{
			Key:      common.NamespaceLabelKey,
			Operator: metav1.LabelSelectorOpIn,
			Values:   ls.config.EnabledNamespaces,
		})
	}

	// AKS automatically adds some selector requirements if we don't so we need to add them to avoid conflicts when
	// updating the webhook. Ref: https://docs.microsoft.com/en-us/azure/aks/faq#can-i-use-admission-controller-webhooks-on-aks
	if ls.config.AddAksSelectors {
		namespaceSelector.MatchExpressions = append(namespaceSelector.MatchExpressions, common.AzureAKSLabelSelectorRequirement()...)
	}

	return namespaceSelector, objectSelector
}

func (ls *LabelSelectors) setupObjectSelector(selector *metav1.LabelSelector) {
	if ls.config.Enabled || ls.config.MutateUnlabelled {
		// If instrumentation or mutate unlabelled is enabled, then we want to receive webhooks for everything but
		// workloads that have explicitly opted out.
		selector.MatchExpressions = []metav1.LabelSelectorRequirement{
			{
				Key:      common.EnabledLabelKey,
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"false"},
			},
		}
	} else {
		// Otherwise, we should only receive webhook requests for workloads that have opted in to mutation.
		selector.MatchLabels = map[string]string{
			common.EnabledLabelKey: "true",
		}
	}
}
