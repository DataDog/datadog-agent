// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver || kubelet

package instrumentation

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// AppliedTargetAnnotation is the JSON of the target that was applied to the pod.
	AppliedTargetAnnotation = "internal.apm.datadoghq.com/applied-target"
)

// Config is a struct to store the configuration for the autoinstrumentation logic. It can be populated
// using the datadog config through NewInstrumentationConfig.
type Config struct {
	// Enabled is a flag to enable the auto instrumentation. If false, the auto instrumentation is disabled with the
	// caveat of the annotation based instrumentation. Full config
	// key: apm_config.instrumentation.enabled
	Enabled bool `mapstructure:"enabled" json:"enabled"`
	// EnabledNamespaces is a list of namespaces where the autoinstrumentation is enabled. If empty, it is enabled in
	// all namespaces. EnabledNamespace and DisabledNamespaces are mutually exclusive and cannot be set together. Full
	// config key: apm_config.instrumentation.enabled_namespaces
	EnabledNamespaces []string `mapstructure:"enabled_namespaces" json:"enabled_namespaces"`
	// DisabledNamespaces is a list of namespaces where the autoinstrumentation is disabled. If empty, it is enabled in
	// all namespaces. EnabledNamespace and DisabledNamespaces are mutually exclusive and cannot be set together. Full
	// config key: apm_config.instrumentation.disabled_namespaces
	DisabledNamespaces []string `mapstructure:"disabled_namespaces" json:"disabled_namespaces"`
	// LibVersions is a map of tracer libraries to inject with their versions. The key is the language and the value is
	// the version of the library to inject. If empty, the auto instrumentation will inject all libraries. Full config
	// key: apm_config.instrumentation.lib_versions
	LibVersions map[string]string `mapstructure:"lib_versions" json:"lib_versions"`
	// Version is the version of the autoinstrumentation logic to use. We don't expose this option to the user, and V1
	// is deprecated and slated for removal. Full config key: apm_config.instrumentation.version
	Version string `mapstructure:"version" json:"version"`
	// InjectorImageTag is the tag of the image to use for the auto instrumentation injector library. Full config key:
	// apm_config.instrumentation.injector_image_tag
	InjectorImageTag string `mapstructure:"injector_image_tag" json:"injector_image_tag"`
	// Targets is a list of targets to apply the auto instrumentation to. The first target that matches the pod will be
	// used. If no target matches, the auto instrumentation will not be applied. Full config key:
	// apm_config.instrumentation.targets
	Targets []Target `mapstructure:"targets" json:"targets"`
}

// Target is a rule to apply the auto instrumentation to a specific workload using the pod and namespace selectors.
// Full config key: apm_config.instrumentation.targets to get the list of targets.
type Target struct {
	// Name is the name of the target. It will be appended to the pod annotations to identify the target that was used.
	// Full config key: apm_config.instrumentation.targets[].name
	Name string `mapstructure:"name" json:"name,omitempty"`
	// PodSelector is the pod selector to match the pods to apply the auto instrumentation to. It will be used in
	// conjunction with the NamespaceSelector to match the pods. Full config key:
	// apm_config.instrumentation.targets[].selector
	PodSelector *PodSelector `mapstructure:"podSelector" json:"podSelector,omitempty"`
	// NamespaceSelector is the namespace selector to match the namespaces to apply the auto instrumentation to. It will
	// be used in conjunction with the Selector to match the pods. Full config key:
	// apm_config.instrumentation.targets[].namespaceSelector
	NamespaceSelector *NamespaceSelector `mapstructure:"namespaceSelector" json:"namespaceSelector,omitempty"`
	// TracerVersions is a map of tracer versions to inject for workloads that match the target. The key is the tracer
	// name and the value is the version to inject. Full config key:
	// apm_config.instrumentation.targets[].ddTraceVersions
	TracerVersions map[string]string `mapstructure:"ddTraceVersions" json:"ddTraceVersions,omitempty"`
	// TracerConfigs is a list of configuration options to use for the installed tracers. These options will be added
	// as environment variables in addition to the injected tracer. Full config key:
	// apm_config.instrumentation.targets[].ddTraceConfigs
	TracerConfigs []TracerConfig `mapstructure:"ddTraceConfigs" json:"ddTraceConfigs,omitempty"`
}

// PodSelector is a reconstruction of the metav1.LabelSelector struct to be able to unmarshal the configuration. It
// can be converted to a metav1.LabelSelector using the AsLabelSelector method. Full config key:
// apm_config.instrumentation.targets[].selector
type PodSelector struct {
	// MatchLabels is a map of key-value pairs to match the labels of the pod. The labels and expressions are ANDed.
	// Full config key: apm_config.instrumentation.targets[].podSelector.matchLabels
	MatchLabels map[string]string `mapstructure:"matchLabels" json:"matchLabels,omitempty"`
	// MatchExpressions is a list of label selector requirements to match the labels of the pod. The labels and
	// expressions are ANDed. Full config key: apm_config.instrumentation.targets[].podSelector.matchExpressions
	MatchExpressions []SelectorMatchExpression `mapstructure:"matchExpressions" json:"matchExpressions,omitempty"`
}

// AsLabelSelector converts the PodSelector to a labels.Selector. It returns an error if the conversion fails.
func (p PodSelector) AsLabelSelector() (labels.Selector, error) {
	labelSelector := &metav1.LabelSelector{
		MatchLabels:      p.MatchLabels,
		MatchExpressions: make([]metav1.LabelSelectorRequirement, len(p.MatchExpressions)),
	}
	for i, expr := range p.MatchExpressions {
		labelSelector.MatchExpressions[i] = metav1.LabelSelectorRequirement{
			Key:      expr.Key,
			Operator: expr.Operator,
			Values:   expr.Values,
		}
	}

	return metav1.LabelSelectorAsSelector(labelSelector)
}

// SelectorMatchExpression is a reconstruction of the metav1.LabelSelectorRequirement struct to be able to unmarshal
// the configuration.
type SelectorMatchExpression struct {
	// Key is the key of the label to match.
	Key string `mapstructure:"key" json:"key,omitempty"`
	// Operator is the operator to use to match the label. Valid values are In, NotIn, Exists, DoesNotExist.
	Operator metav1.LabelSelectorOperator `mapstructure:"operator" json:"operator,omitempty"`
	// Values is a list of values to match the label against. If the operator is Exists or DoesNotExist, the values
	// should be empty. If the operator is In or NotIn, the values should be non-empty.
	Values []string `mapstructure:"values" json:"values,omitempty"`
}

// NamespaceSelector is a struct to store the configuration for the namespace selector. It can be used to match the
// namespaces to apply the auto instrumentation to. Full config key:
// apm_config.instrumentation.targets[].namespaceSelector
type NamespaceSelector struct {
	// MatchNames is a list of namespace names to match. If empty, all namespaces are matched. Full config key:
	// apm_config.instrumentation.targets[].namespaceSelector.matchNames
	MatchNames []string `mapstructure:"matchNames" json:"matchNames,omitempty"`
	// MatchLabels is a map of key-value pairs to match the labels of the namespace. The labels and expressions are
	// ANDed. This cannot be used with MatchNames. Full config key:
	// apm_config.instrumentation.targets[].namespaceSelector.matchLabels
	MatchLabels map[string]string `mapstructure:"matchLabels" json:"matchLabels,omitempty"`
	// MatchExpressions is a list of label selector requirements to match the labels of the namespace. The labels and
	// expressions are ANDed. This cannot be used with MatchNames. Full config key:
	// apm_config.instrumentation.targets[].selector.matchExpressions
	MatchExpressions []SelectorMatchExpression `mapstructure:"matchExpressions" json:"matchExpressions,omitempty"`
}

// AsLabelSelector converts the NamespaceSelector to a labels.Selector. It returns an error if the conversion fails.
func (n NamespaceSelector) AsLabelSelector() (labels.Selector, error) {
	labelSelector := &metav1.LabelSelector{
		MatchLabels:      n.MatchLabels,
		MatchExpressions: make([]metav1.LabelSelectorRequirement, len(n.MatchExpressions)),
	}
	for i, expr := range n.MatchExpressions {
		labelSelector.MatchExpressions[i] = metav1.LabelSelectorRequirement{
			Key:      expr.Key,
			Operator: expr.Operator,
			Values:   expr.Values,
		}
	}

	return metav1.LabelSelectorAsSelector(labelSelector)
}

// TracerConfig is a struct that stores configuration options for a tracer. These will be injected as environment
// variables to the workload that matches targeting.
type TracerConfig struct {
	// Name is the name of the environment variable.
	Name string `json:"name,omitempty"`
	// Value is the value to use.
	Value string `json:"value,omitempty"`
	// ValueFrom is the source for the environment variable's value.
	ValueFrom *corev1.EnvVarSource `json:"valueFrom,omitempty"`
}

// AsEnvVar converts the TracerConfig to a corev1.EnvVar.
func (c *TracerConfig) AsEnvVar() corev1.EnvVar {
	return corev1.EnvVar{
		Name:      c.Name,
		Value:     c.Value,
		ValueFrom: c.ValueFrom,
	}
}
