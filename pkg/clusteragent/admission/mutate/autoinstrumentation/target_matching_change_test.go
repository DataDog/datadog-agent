// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import "testing"

// TestMatching_UnresolvableNamespaceRulePreservesLegacyOrdering verifies that
// an unavailable namespace-label source aborts matching only once evaluation
// reaches a policy that needs it. An earlier matching policy still wins.
func TestMatching_UnresolvableNamespaceRulePreservesLegacyOrdering(t *testing.T) {
	const podRuleFirst = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "pod-only"
        podSelector:
          matchLabels:
            app: "web"
        ddTraceVersions:
          java: "default"
      - name: "ns-label"
        namespaceSelector:
          matchLabels:
            instrument: "true"
        ddTraceVersions:
          python: "default"
`
	const nsRuleFirst = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "ns-label"
        namespaceSelector:
          matchLabels:
            instrument: "true"
        ddTraceVersions:
          python: "default"
      - name: "pod-only"
        podSelector:
          matchLabels:
            app: "web"
        ddTraceVersions:
          java: "default"
`
	// No namespace is registered in the store, so the namespace-label rule
	// cannot be evaluated for the "ghost" namespace.
	runMatchCases(t, podRuleFirst, []matchCase{
		{name: "pod-only rule before the unresolvable rule still matches", ns: "ghost", podLabels: map[string]string{"app": "web"}, want: "pod-only"},
	})
	runMatchCases(t, nsRuleFirst, []matchCase{
		{name: "unresolvable rule first aborts before the pod-only rule", ns: "ghost", podLabels: map[string]string{"app": "web"}, want: ""},
	})
}
