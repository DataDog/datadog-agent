// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import "testing"

// TestMatching_NamespaceMissingFromStore documents an intentional behavior change
// introduced by the policy-engine matcher.
//
// When a pod's namespace is absent from the workloadmeta store and at least one
// target reads namespace labels, the matcher cannot resolve the facts it needs
// and declines to inject (fail-safe: no injection on an unresolved namespace).
//
// The legacy label-selector matcher was order-dependent here: a pod-only target
// placed before the namespace-label target would match without ever resolving
// namespace labels, so the same pod would be injected. The policy matcher
// resolves namespace labels once, up front, which removes that order dependence
// at the cost of not matching the earlier pod-only target in this case. This is
// considered an improvement (consistent, order-independent fail-safe) and is
// pinned here so the behavior is explicit and intentional rather than accidental.
func TestMatching_NamespaceMissingFromStore(t *testing.T) {
	const cfg = `
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
	// No namespace is registered in the store, so namespace-label resolution
	// fails for the "ghost" namespace and matching is aborted.
	runMatchCases(t, cfg, []matchCase{
		{name: "unresolved namespace declines injection even for an earlier pod-only target", ns: "ghost", podLabels: map[string]string{"app": "web"}, want: ""},
	})
}
