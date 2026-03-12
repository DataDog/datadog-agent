// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workloadconfig

import (
	"fmt"
	"sort"
	"strings"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// BuildCELSelector builds a CEL selector from a DatadogInstrumentation's Selector and namespace.
func BuildCELSelector(dwc *datadoghq.DatadogInstrumentation) workloadfilter.Rules {
	hasAnnotations := len(dwc.Spec.Selector.MatchAnnotations) > 0
	hasLabels := len(dwc.Spec.Selector.MatchLabels) > 0
	var rules []string
	if hasAnnotations {
		rules = append(rules, BuildMapCELRules("container.pod.annotations", dwc.Spec.Selector.MatchAnnotations)...)
	}
	if hasLabels {
		rules = append(rules, BuildMapCELRules("container.pod.labels", dwc.Spec.Selector.MatchLabels)...)
	}
	rules = append(rules, fmt.Sprintf("container.pod.namespace == '%s'", dwc.Namespace))
	combinedRules := strings.Join(rules, " && ")
	return workloadfilter.Rules{Containers: []string{combinedRules}}
}

// BuildMapCELRules generates CEL equality expressions for a map field.
// Each key-value pair becomes: fieldPath["key"] == "value".
// Keys are sorted for deterministic output.
func BuildMapCELRules(fieldPath string, m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rules := make([]string, 0, len(m))
	for _, k := range keys {
		rules = append(rules, fmt.Sprintf(`%s["%s"] == "%s"`, fieldPath, k, m[k]))
	}
	return rules
}
