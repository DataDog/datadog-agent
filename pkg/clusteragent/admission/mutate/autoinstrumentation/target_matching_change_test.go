// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import "testing"

// TestMatching_UnresolvableNamespaceRuleIsSkipped documents an intentional
// behavior change introduced by the policy-engine matcher.
//
// When a pod's namespace is absent from the workloadmeta store, a rule that
// reads namespace labels cannot be evaluated. The matcher now ignores only that
// rule and keeps evaluating the remaining rules in order, so an unresolvable
// namespace rule never blocks an otherwise-matching rule from injecting.
//
// The legacy label-selector matcher instead aborted all matching as soon as it
// reached a rule whose namespace could not be resolved (returning "no match").
// It only injected in this situation by accident of ordering: if a matching
// pod-only rule happened to come first, it short-circuited before the
// unresolvable rule was reached. These cases pin the new, order-independent
// behavior; the "rule first" case is the one that changes versus the legacy
// matcher (it would previously have aborted).
func TestMatching_UnresolvableNamespaceRuleIsSkipped(t *testing.T) {
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
	// cannot be evaluated for the "ghost" namespace and is skipped.
	runMatchCases(t, podRuleFirst, []matchCase{
		{name: "pod-only rule before the unresolvable rule still matches", ns: "ghost", podLabels: map[string]string{"app": "web"}, want: "pod-only"},
	})
	runMatchCases(t, nsRuleFirst, []matchCase{
		{name: "unresolvable rule first does not block the pod-only rule", ns: "ghost", podLabels: map[string]string{"app": "web"}, want: "pod-only"},
	})
}
