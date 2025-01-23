// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoinstrumentation

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/DataDog/datadog-agent/comp/core/config"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type TargetFilter struct {
	targets               []Target
	containerRegistry     string
	disabledNamespaces    map[string]bool
	defaultTracerVersions []libInfo
}

func NewTargetFilter(datadogConfig config.Component) (*TargetFilter, error) {
	targets, err := ParseConfig(datadogConfig)
	if err != nil {
		return nil, fmt.Errorf("error parsing targets from config: %w", err)
	}

	disabledNamespacesCfg := datadogConfig.GetStringSlice("apm_config.instrumentation.disabled_namespaces")
	disabledNamespaces := make(map[string]bool, len(disabledNamespacesCfg))
	for _, ns := range disabledNamespacesCfg {
		disabledNamespaces[ns] = true
	}

	// TODO: pass this in as a parameter.
	containerRegistry := mutatecommon.ContainerRegistry(datadogConfig, "admission_controller.auto_instrumentation.container_registry")
	defaultTracerVersions := getAllLatestDefaultLibraries(containerRegistry)

	return &TargetFilter{
		targets:               targets,
		disabledNamespaces:    disabledNamespaces,
		containerRegistry:     containerRegistry,
		defaultTracerVersions: defaultTracerVersions,
	}, nil
}

func (f *TargetFilter) Filter(pod *corev1.Pod) []libInfo {
	// If the namespace is disabled, we don't need to check the targets.
	if _, ok := f.disabledNamespaces[pod.Namespace]; ok {
		return nil
	}

	// Check if the pod matches any of the targets. The first match wins.
	for _, target := range f.targets {
		// Check the pod namespace against the namespace selector.
		if !matchesNamespaceSelector(pod, target.NamespaceSelector) {
			log.Debugf("Pod ns %s does not match namespace selector %v", pod.Namespace, target.NamespaceSelector)
			continue
		}

		// Check the pod labels against the pod selector.
		if !target.Selector.podSelector.Matches(labels.Set(pod.Labels)) {
			continue
		}

		// If no tracer versions are specified, use the default.
		if len(target.TracerVersions) == 0 {
			return f.defaultTracerVersions
		}

		// Otherwise, use the configured tracers.
		return target.tracerVersions
	}

	// No target matched.
	return nil
}

func matchesNamespaceSelector(pod *corev1.Pod, selector NamespaceSelector) bool {
	// If there are no match names, the selector matches all namespaces.
	if len(selector.MatchNames) == 0 {
		return true
	}

	// Check if the pod namespace is in the match names.
	_, ok := selector.matchNamesMapped[pod.Namespace]
	return ok
}

func ParseConfig(datadogConfig config.Component) ([]Target, error) {
	containerRegistry := mutatecommon.ContainerRegistry(datadogConfig, "admission_controller.auto_instrumentation.container_registry")

	data := datadogConfig.Get("apm_config.instrumentation.targets")
	if data == nil {
		return nil, nil
	}

	targets := []Target{}
	err := mapstructure.Decode(data, &targets)
	if err != nil {
		return nil, err
	}

	// Preprocess config for faster filtering, which happens on every pod.
	for i := range targets {
		// Preprocess the namespace selector.
		targets[i].NamespaceSelector.matchNamesMapped = make(map[string]bool, len(targets[i].NamespaceSelector.MatchNames))
		for _, ns := range targets[i].NamespaceSelector.MatchNames {
			targets[i].NamespaceSelector.matchNamesMapped[ns] = true
		}

		// Preprocess the pod selector.
		podSelector, err := targets[i].Selector.AsLabelSelector()
		if err != nil {
			return nil, fmt.Errorf("error parsing pod selector for target %s: %w", targets[i].Name, err)
		}
		targets[i].Selector.podSelector = podSelector

		tracers := []libInfo{}
		for lang, version := range targets[i].TracerVersions {
			l := language(lang)
			if !l.isSupported() {
				log.Warnf("APM Instrumentation detected configuration for unsupported language: %s. Tracing library for %s will not be injected", lang, lang)
				continue
			}

			log.Infof("Library version %s is specified for language %s", version, lang)
			tracers = append(tracers, l.libInfo("", l.libImageName(containerRegistry, version)))
		}
		targets[i].tracerVersions = tracers
	}

	return targets, nil
}

type Target struct {
	Name              string            `mapstructure:"name"`
	Selector          PodSelector       `mapstructure:"selector"`
	NamespaceSelector NamespaceSelector `mapstructure:"namespaceSelector"`
	TracerVersions    map[string]string `mapstructure:"ddTraceVersions"`
	tracerVersions    []libInfo
}

type PodSelector struct {
	// metav1.LabelSelector
	MatchLabels      map[string]string            `mapstructure:"matchLabels"`
	MatchExpressions []PodSelectorMatchExpression `mapstructure:"matchExpressions"`
	podSelector      labels.Selector
}

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

type PodSelectorMatchExpression struct {
	// metav1.LabelSelectorRequirement
	Key      string                       `mapstructure:"key"`
	Operator metav1.LabelSelectorOperator `mapstructure:"operator"`
	Values   []string                     `mapstructure:"values"`
}

type NamespaceSelector struct {
	MatchNames []string `mapstructure:"matchNames"`

	matchNamesMapped map[string]bool
}
