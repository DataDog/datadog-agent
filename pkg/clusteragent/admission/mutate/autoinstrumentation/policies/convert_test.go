// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package policies

import "testing"

func decideName(ps []Policy, f Facts) string {
	for i := range ps {
		if Evaluate(ps[i].Rules, f) == ResultTrue {
			return ps[i].Name
		}
	}
	return ""
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
	ps := FromTargets(ts)

	if got := decideName(ps, Facts{PodLabels: map[string]string{"app": "db-user"}}); got != "db-user" {
		t.Errorf("db-user pod matched %q", got)
	}
	if got := decideName(ps, Facts{PodLabels: map[string]string{"app": "user-request-router", "webserver": "user"}}); got != "user-request-router" {
		t.Errorf("router pod matched %q", got)
	}
	// matchLabels are ANDed: missing webserver label must not match the router.
	if got := decideName(ps, Facts{PodLabels: map[string]string{"app": "user-request-router"}}); got != "" {
		t.Errorf("partial router pod matched %q want none", got)
	}
	if got := decideName(ps, Facts{PodLabels: map[string]string{"app": "other"}}); got != "" {
		t.Errorf("unrelated pod matched %q want none", got)
	}
}

func TestConvertNotIn(t *testing.T) {
	ts := []Target{{
		Name: "all-but",
		PodSelector: &PodSelector{MatchExpressions: []LabelSelectorRequirement{{
			Key:      "app",
			Operator: OpNotIn,
			Values:   []string{"app1", "app2"},
		}}},
	}}
	ps := FromTargets(ts)

	if got := decideName(ps, Facts{PodLabels: map[string]string{"app": "app3"}}); got != "all-but" {
		t.Errorf("app3 matched %q want all-but", got)
	}
	if got := decideName(ps, Facts{PodLabels: map[string]string{"app": "app1"}}); got != "" {
		t.Errorf("app1 matched %q want none", got)
	}
	// NotIn matches absent keys (Kubernetes semantics).
	if got := decideName(ps, Facts{PodLabels: map[string]string{}}); got != "all-but" {
		t.Errorf("absent label matched %q want all-but", got)
	}
}

func TestConvertInAndExists(t *testing.T) {
	ts := []Target{{
		Name: "expr",
		PodSelector: &PodSelector{MatchExpressions: []LabelSelectorRequirement{
			{Key: "lang", Operator: OpIn, Values: []string{"java", "go"}},
			{Key: "tier", Operator: OpExists},
			{Key: "deprecated", Operator: OpDoesNotExist},
		}},
	}}
	ps := FromTargets(ts)

	match := Facts{PodLabels: map[string]string{"lang": "go", "tier": "frontend"}}
	if got := decideName(ps, match); got != "expr" {
		t.Errorf("expected match, got %q", got)
	}
	// lang not in set
	if got := decideName(ps, Facts{PodLabels: map[string]string{"lang": "ruby", "tier": "x"}}); got != "" {
		t.Errorf("ruby matched %q want none", got)
	}
	// tier missing
	if got := decideName(ps, Facts{PodLabels: map[string]string{"lang": "go"}}); got != "" {
		t.Errorf("missing tier matched %q want none", got)
	}
	// deprecated present -> DoesNotExist fails
	if got := decideName(ps, Facts{PodLabels: map[string]string{"lang": "go", "tier": "x", "deprecated": "true"}}); got != "" {
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
	ps := FromTargets(ts)

	if got := decideName(ps, Facts{NamespaceName: "billing"}); got != "by-name" {
		t.Errorf("billing ns matched %q want by-name", got)
	}
	if got := decideName(ps, Facts{NamespaceName: "default"}); got != "" {
		t.Errorf("default ns matched %q want none", got)
	}
	matched := Facts{
		NamespaceName:   "default",
		NamespaceLabels: map[string]string{"instrument": "true"},
		PodLabels:       map[string]string{"app": "web"},
	}
	if got := decideName(ps, matched); got != "by-label" {
		t.Errorf("labeled ns pod matched %q want by-label", got)
	}
}

func TestConvertEmptyTargetMatchesEverything(t *testing.T) {
	ps := FromTargets([]Target{{Name: "default", TracerVersions: map[string]string{"java": "latest"}}})
	out, ok := Decide(ps, Facts{NamespaceName: "anything", PodLabels: map[string]string{"x": "y"}})
	if !ok || out.TracerVersions["java"] != "latest" {
		t.Fatalf("empty target should match everything, got %+v ok=%v", out, ok)
	}
}
