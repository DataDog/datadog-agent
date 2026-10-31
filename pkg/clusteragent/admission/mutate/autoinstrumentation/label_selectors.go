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

// LabelSelectorsConfig provides configuration values for NewLabelSelectors.
type LabelSelectorsConfig struct {
	DisabledNamespaces []string
}

// NewLabelSelectorsConfig initializes a config object from the datadog config.
func NewLabelSelectorsConfig(datadogConfig config.Component) *LabelSelectorsConfig {
	return &LabelSelectorsConfig{
		DisabledNamespaces: datadogConfig.GetStringSlice("apm_config.instrumentation.disabled_namespaces"),
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

	return namespaceSelector, objectSelector
}

func (ls *LabelSelectors) setupObjectSelector(selector *metav1.LabelSelector) {
	// Receive webhooks for everything but workloads that have explicitly opted out. Each injection source decides
	// whether to act on a pod (e.g. the annotation source requires the enabled label unless mutate_unlabelled is set),
	// so the selector does not need to track which features are enabled.
	selector.MatchExpressions = []metav1.LabelSelectorRequirement{
		{
			Key:      common.EnabledLabelKey,
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   []string{"false"},
		},
	}
}
