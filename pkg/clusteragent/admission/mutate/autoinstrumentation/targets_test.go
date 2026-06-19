// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/dd-policy-engine/go/policies"
)

func decideName(ps []policies.Policy, f policies.Facts) string {
	for i := range ps {
		if policies.Evaluate(ps[i].Rules, f) == policies.ResultTrue {
			return ps[i].Name
		}
	}
	return ""
}

func TestPoliciesFromTargetsOutcome(t *testing.T) {
	targets := []Target{{
		Name:        "java",
		PodSelector: &PodSelector{MatchLabels: map[string]string{"app": "db"}},
		NamespaceSelector: &NamespaceSelector{
			MatchExpressions: []SelectorMatchExpression{{
				Key:      "team",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"payments"},
			}},
		},
		TracerVersions: map[string]string{"java": "latest"},
		TracerConfigs:  []TracerConfig{{Name: "DD_PROFILING_ENABLED", Value: "true"}},
	}}

	got := policiesFromTargets(targets)
	if len(got) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(got))
	}
	p := got[0]
	if p.Name != "java" || !p.Outcome.Inject {
		t.Errorf("unexpected policy identity/outcome: %+v", p)
	}
	if p.Outcome.TracerVersions["java"] != "latest" {
		t.Errorf("tracer versions not mapped: %+v", p.Outcome.TracerVersions)
	}
	if len(p.Outcome.TracerConfigs) != 1 || p.Outcome.TracerConfigs[0] != (policies.EnvVar{Name: "DD_PROFILING_ENABLED", Value: "true"}) {
		t.Errorf("tracer config not mapped: %+v", p.Outcome.TracerConfigs)
	}
}

// TestConvertPodLabelTargets reproduces the canonical SSI example: one target
// for the "db-user" app (java) and one for the "user-request-router" with a
// webserver=user pod label (php), in order.
func TestConvertPodLabelTargets(t *testing.T) {
	ts := []Target{
		{
			Name:           "db-user",
			PodSelector:    &PodSelector{MatchLabels: map[string]string{"app": "db-user"}},
			TracerVersions: map[string]string{"java": "latest"},
		},
		{
			Name: "user-request-router",
			PodSelector: &PodSelector{MatchLabels: map[string]string{
				"app":       "user-request-router",
				"webserver": "user",
			}},
			TracerVersions: map[string]string{"php": "latest"},
		},
	}
	ps := policiesFromTargets(ts)

	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{"app": "db-user"}}); got != "db-user" {
		t.Errorf("db-user pod matched %q", got)
	}
	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{"app": "user-request-router", "webserver": "user"}}); got != "user-request-router" {
		t.Errorf("router pod matched %q", got)
	}
	// matchLabels are ANDed: missing webserver label must not match the router.
	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{"app": "user-request-router"}}); got != "" {
		t.Errorf("partial router pod matched %q want none", got)
	}
	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{"app": "other"}}); got != "" {
		t.Errorf("unrelated pod matched %q want none", got)
	}
}

func TestConvertNotIn(t *testing.T) {
	ts := []Target{{
		Name: "all-but",
		PodSelector: &PodSelector{MatchExpressions: []SelectorMatchExpression{{
			Key:      "app",
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   []string{"app1", "app2"},
		}}},
	}}
	ps := policiesFromTargets(ts)

	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{"app": "app3"}}); got != "all-but" {
		t.Errorf("app3 matched %q want all-but", got)
	}
	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{"app": "app1"}}); got != "" {
		t.Errorf("app1 matched %q want none", got)
	}
	// NotIn matches absent keys (Kubernetes semantics).
	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{}}); got != "all-but" {
		t.Errorf("absent label matched %q want all-but", got)
	}
}

func TestConvertInAndExists(t *testing.T) {
	ts := []Target{{
		Name: "expr",
		PodSelector: &PodSelector{MatchExpressions: []SelectorMatchExpression{
			{Key: "lang", Operator: metav1.LabelSelectorOpIn, Values: []string{"java", "go"}},
			{Key: "tier", Operator: metav1.LabelSelectorOpExists},
			{Key: "deprecated", Operator: metav1.LabelSelectorOpDoesNotExist},
		}},
	}}
	ps := policiesFromTargets(ts)

	match := policies.Facts{PodLabels: map[string]string{"lang": "go", "tier": "frontend"}}
	if got := decideName(ps, match); got != "expr" {
		t.Errorf("expected match, got %q", got)
	}
	// lang not in set
	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{"lang": "ruby", "tier": "x"}}); got != "" {
		t.Errorf("ruby matched %q want none", got)
	}
	// tier missing
	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{"lang": "go"}}); got != "" {
		t.Errorf("missing tier matched %q want none", got)
	}
	// deprecated present -> DoesNotExist fails
	if got := decideName(ps, policies.Facts{PodLabels: map[string]string{"lang": "go", "tier": "x", "deprecated": "true"}}); got != "" {
		t.Errorf("deprecated matched %q want none", got)
	}
}

func TestConvertNamespaceSelectors(t *testing.T) {
	ts := []Target{
		{
			Name:              "by-name",
			NamespaceSelector: &NamespaceSelector{MatchNames: []string{"payments", "billing"}},
			TracerVersions:    map[string]string{"java": "latest"},
		},
		{
			Name:              "by-label",
			NamespaceSelector: &NamespaceSelector{MatchLabels: map[string]string{"instrument": "true"}},
			PodSelector:       &PodSelector{MatchLabels: map[string]string{"app": "web"}},
		},
	}
	ps := policiesFromTargets(ts)

	if got := decideName(ps, policies.Facts{NamespaceName: "billing"}); got != "by-name" {
		t.Errorf("billing ns matched %q want by-name", got)
	}
	if got := decideName(ps, policies.Facts{NamespaceName: "default"}); got != "" {
		t.Errorf("default ns matched %q want none", got)
	}
	matched := policies.Facts{
		NamespaceName:   "default",
		NamespaceLabels: map[string]string{"instrument": "true"},
		PodLabels:       map[string]string{"app": "web"},
	}
	if got := decideName(ps, matched); got != "by-label" {
		t.Errorf("labeled ns pod matched %q want by-label", got)
	}
}

func TestConvertEmptyTargetMatchesEverything(t *testing.T) {
	ps := policiesFromTargets([]Target{{Name: "default", TracerVersions: map[string]string{"java": "latest"}}})
	out, ok := policies.Decide(ps, policies.Facts{NamespaceName: "anything", PodLabels: map[string]string{"x": "y"}})
	if !ok || out.TracerVersions["java"] != "latest" {
		t.Fatalf("empty target should match everything, got %+v ok=%v", out, ok)
	}
}

// TestTargetMatchingDocExamples is a behavioral guardrail for the targets path:
// it mirrors every example from the public "Target specific workloads"
// documentation plus the selector building blocks that the examples do not
// cover (namespace matchExpressions, combined namespace+pod selectors, and the
// "first match wins" ordering rule). It exercises the same targets -> policies
// lowering and evaluation that drives the cluster-agent at runtime, so any
// regression in matching behavior is caught here.
func TestTargetMatchingDocExamples(t *testing.T) {
	// Example 2: namespace by name, then namespace by label.
	doc2 := []Target{
		{
			Name:              "login-service_namespace",
			NamespaceSelector: &NamespaceSelector{MatchNames: []string{"login-service"}},
			TracerVersions:    map[string]string{"java": "default"},
			TracerConfigs:     []TracerConfig{{Name: "DD_PROFILING_ENABLED", Value: "auto"}},
		},
		{
			Name:              "billing-service_apps",
			NamespaceSelector: &NamespaceSelector{MatchLabels: map[string]string{"app": "billing-service"}},
			TracerVersions:    map[string]string{"python": "3.1.0"},
		},
	}
	// Example 3: two pod-label targets (also exercises first-match-wins).
	doc3 := []Target{
		{
			Name:           "db-user",
			PodSelector:    &PodSelector{MatchLabels: map[string]string{"app": "db-user"}},
			TracerVersions: map[string]string{"java": "default"},
		},
		{
			Name:           "user-request-router",
			PodSelector:    &PodSelector{MatchLabels: map[string]string{"webserver": "user"}},
			TracerVersions: map[string]string{"php": "default"},
		},
	}
	// Example 4: a pod within a namespace (namespace matchNames AND pod matchLabels).
	doc4 := []Target{{
		Name:              "login-service-namespace",
		NamespaceSelector: &NamespaceSelector{MatchNames: []string{"login-service"}},
		PodSelector:       &PodSelector{MatchLabels: map[string]string{"app": "password-resolver"}},
		TracerVersions:    map[string]string{"java": "default"},
	}}
	// Example 5: a subset of pods using matchExpressions (NotIn).
	doc5 := []Target{{
		Name: "default-target",
		PodSelector: &PodSelector{MatchExpressions: []SelectorMatchExpression{{
			Key:      "app",
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   []string{"app1", "app2"},
		}}},
	}}
	// Namespace matchExpressions: not shown in the doc examples but a supported
	// building block, mirrored for both In and Exists.
	nsExpr := []Target{{
		Name: "ns-expr",
		NamespaceSelector: &NamespaceSelector{MatchExpressions: []SelectorMatchExpression{
			{Key: "team", Operator: metav1.LabelSelectorOpIn, Values: []string{"payments", "billing"}},
			{Key: "instrument", Operator: metav1.LabelSelectorOpExists},
		}},
	}}

	cases := []struct {
		name    string
		targets []Target
		facts   policies.Facts
		want    string // matched target name, "" for no match
	}{
		// Example 1: no selectors instruments everything.
		{
			name:    "ex1 no-selector matches any pod",
			targets: []Target{{Name: "all-remaining-services", TracerVersions: map[string]string{"java": "default"}}},
			facts:   policies.Facts{NamespaceName: "whatever", PodLabels: map[string]string{"any": "thing"}},
			want:    "all-remaining-services",
		},
		// Example 2.
		{name: "ex2 namespace matchNames", targets: doc2, facts: policies.Facts{NamespaceName: "login-service"}, want: "login-service_namespace"},
		{name: "ex2 namespace matchLabels", targets: doc2, facts: policies.Facts{NamespaceName: "x", NamespaceLabels: map[string]string{"app": "billing-service"}}, want: "billing-service_apps"},
		{name: "ex2 no match", targets: doc2, facts: policies.Facts{NamespaceName: "other"}, want: ""},
		// Example 3.
		{name: "ex3 db-user", targets: doc3, facts: policies.Facts{PodLabels: map[string]string{"app": "db-user"}}, want: "db-user"},
		{name: "ex3 router", targets: doc3, facts: policies.Facts{PodLabels: map[string]string{"webserver": "user"}}, want: "user-request-router"},
		{name: "ex3 first match wins when both labels present", targets: doc3, facts: policies.Facts{PodLabels: map[string]string{"app": "db-user", "webserver": "user"}}, want: "db-user"},
		{name: "ex3 no match", targets: doc3, facts: policies.Facts{PodLabels: map[string]string{"app": "other"}}, want: ""},
		// Example 4: namespace AND pod selectors are ANDed.
		{name: "ex4 namespace and pod match", targets: doc4, facts: policies.Facts{NamespaceName: "login-service", PodLabels: map[string]string{"app": "password-resolver"}}, want: "login-service-namespace"},
		{name: "ex4 pod mismatch in matching namespace", targets: doc4, facts: policies.Facts{NamespaceName: "login-service", PodLabels: map[string]string{"app": "other"}}, want: ""},
		{name: "ex4 matching pod in wrong namespace", targets: doc4, facts: policies.Facts{NamespaceName: "other", PodLabels: map[string]string{"app": "password-resolver"}}, want: ""},
		// Example 5: NotIn, including the Kubernetes "absent key matches" rule.
		{name: "ex5 notin allows other", targets: doc5, facts: policies.Facts{PodLabels: map[string]string{"app": "app3"}}, want: "default-target"},
		{name: "ex5 notin excludes listed", targets: doc5, facts: policies.Facts{PodLabels: map[string]string{"app": "app1"}}, want: ""},
		{name: "ex5 notin matches absent key", targets: doc5, facts: policies.Facts{PodLabels: map[string]string{}}, want: "default-target"},
		// Namespace matchExpressions (In + Exists ANDed).
		{name: "ns-expr in and exists match", targets: nsExpr, facts: policies.Facts{NamespaceName: "x", NamespaceLabels: map[string]string{"team": "payments", "instrument": "yes"}}, want: "ns-expr"},
		{name: "ns-expr in fails", targets: nsExpr, facts: policies.Facts{NamespaceName: "x", NamespaceLabels: map[string]string{"team": "other", "instrument": "yes"}}, want: ""},
		{name: "ns-expr exists fails", targets: nsExpr, facts: policies.Facts{NamespaceName: "x", NamespaceLabels: map[string]string{"team": "payments"}}, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ps := policiesFromTargets(tc.targets)
			if got := decideName(ps, tc.facts); got != tc.want {
				t.Errorf("matched %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTargetMatchingDocExample6Outcome verifies that a matched target returns
// its full injection outcome (tracer versions and ddTraceConfigs), mirroring
// documentation Example 6 (enable AAP + profiler via ddTraceConfigs).
func TestTargetMatchingDocExample6Outcome(t *testing.T) {
	targets := []Target{{
		Name:              "web-apps-with-security",
		NamespaceSelector: &NamespaceSelector{MatchNames: []string{"web-apps"}},
		TracerVersions:    map[string]string{"java": "default", "python": "default"},
		TracerConfigs: []TracerConfig{
			{Name: "DD_APPSEC_ENABLED", Value: "true"},
			{Name: "DD_PROFILING_ENABLED", Value: "auto"},
		},
	}}
	ps := policiesFromTargets(targets)

	out, ok := policies.Decide(ps, policies.Facts{NamespaceName: "web-apps"})
	if !ok {
		t.Fatal("web-apps namespace should match")
	}
	if out.TracerVersions["java"] != "default" || out.TracerVersions["python"] != "default" {
		t.Errorf("unexpected tracer versions: %+v", out.TracerVersions)
	}
	wantEnv := map[string]string{"DD_APPSEC_ENABLED": "true", "DD_PROFILING_ENABLED": "auto"}
	got := make(map[string]string, len(out.TracerConfigs))
	for _, e := range out.TracerConfigs {
		got[e.Name] = e.Value
	}
	for k, v := range wantEnv {
		if got[k] != v {
			t.Errorf("env %q = %q, want %q", k, got[k], v)
		}
	}

	// A pod outside the targeted namespace is not instrumented.
	if _, ok := policies.Decide(ps, policies.Facts{NamespaceName: "other"}); ok {
		t.Error("other namespace should not match")
	}
}
