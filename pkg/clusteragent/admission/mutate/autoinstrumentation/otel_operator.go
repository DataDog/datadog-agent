// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	// otelOperatorInjectAnnotationPrefix prefixes the OpenTelemetry Operator annotations requesting
	// instrumentation, such as instrumentation.opentelemetry.io/inject-java.
	otelOperatorInjectAnnotationPrefix = "instrumentation.opentelemetry.io/inject-"
	// otelOperatorContainerNamesSuffix ends the OpenTelemetry Operator annotations selecting containers rather than
	// requesting instrumentation. instrumentation.opentelemetry.io/inject-nginx-container-names is the only one that
	// also carries the inject- prefix.
	otelOperatorContainerNamesSuffix = "-container-names"
)

// otelOperatorSource vetoes Single Step Instrumentation for pods that request instrumentation from the OpenTelemetry
// Operator, so that remote-config policies, static targets and inject-all do not instrument them a second time.
// Explicit Datadog opt-ins are not vetoed: the annotation and DatadogInstrumentation sources are evaluated first, and
// the sources flagged explicitOptIn (GPU, APM Basic) are exempt.
type otelOperatorSource struct {
	enabled bool
}

func (s *otelOperatorSource) resolve(pod *corev1.Pod) sourceResult {
	if !s.enabled {
		return sourceResult{action: sourcePass}
	}

	requests := otelOperatorInjectRequests(pod.Annotations)
	if len(requests) == 0 {
		return sourceResult{action: sourcePass}
	}

	// The plan carries a per-pod reason, so it is allocated for every resolution.
	return sourceResult{action: sourceVeto, plan: &injectionPlan{
		blocked:    true,
		skipReason: fmt.Sprintf("pod requests instrumentation from the OpenTelemetry Operator (%s)", strings.Join(requests, ", ")),
	}}
}

// otelOperatorInjectRequests returns the annotations requesting instrumentation from the OpenTelemetry Operator,
// formatted as key=value and sorted so that the reason recorded on the pod is the same on every resolution.
//
// It reads every instrumentation.opentelemetry.io/inject-* key except the container selectors, so that languages added
// to the operator are covered, and, like the operator, treats "" and "false" (in any case, not trimmed) as no request.
// Only the pod's own annotations are checked: namespace annotations, and whether the operator is installed, has the
// language enabled or finds an Instrumentation resource, are not.
func otelOperatorInjectRequests(annotations map[string]string) []string {
	var requests []string
	for key, value := range annotations {
		if len(key) == len(otelOperatorInjectAnnotationPrefix) ||
			!strings.HasPrefix(key, otelOperatorInjectAnnotationPrefix) ||
			strings.HasSuffix(key, otelOperatorContainerNamesSuffix) {
			continue
		}
		if value == "" || strings.EqualFold(value, "false") {
			continue
		}
		requests = append(requests, key+"="+value)
	}
	slices.Sort(requests)
	return requests
}
