// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/policies"
)

func podWith(ns string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Labels:    labels,
		},
	}
}

func TestToPolicyTargets(t *testing.T) {
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

	got := toPolicyTargets(targets)
	if len(got) != 1 {
		t.Fatalf("expected 1 policy target, got %d", len(got))
	}
	pt := got[0]
	if pt.Name != "java" || pt.PodSelector.MatchLabels["app"] != "db" {
		t.Errorf("unexpected pod selector mapping: %+v", pt.PodSelector)
	}
	if pt.NamespaceSelector.MatchExpressions[0].Operator != policies.OpIn {
		t.Errorf("operator not mapped: %+v", pt.NamespaceSelector.MatchExpressions[0])
	}
	if pt.TracerConfigs[0] != (policies.EnvVar{Name: "DD_PROFILING_ENABLED", Value: "true"}) {
		t.Errorf("tracer config not mapped: %+v", pt.TracerConfigs)
	}
}

func TestPolicyMatcherPodLabels(t *testing.T) {
	targets := []Target{
		{
			Name:           "java",
			PodSelector:    &PodSelector{MatchLabels: map[string]string{"app": "db"}},
			TracerVersions: map[string]string{"java": "latest"},
		},
		{
			Name:           "catch-all",
			TracerVersions: map[string]string{"php": "latest"},
		},
	}

	m := newPolicyMatcher(targets, nil)
	if m.needsNamespaceLabels {
		t.Errorf("matcher should not require namespace labels for pod-label targets")
	}

	out, ok := m.Match(podWith("any", map[string]string{"app": "db"}))
	if !ok || out.TracerVersions["java"] != "latest" {
		t.Fatalf("db pod: got %+v ok=%v", out, ok)
	}

	out, ok = m.Match(podWith("any", map[string]string{"app": "web"}))
	if !ok || out.TracerVersions["php"] != "latest" {
		t.Fatalf("web pod should hit catch-all: got %+v ok=%v", out, ok)
	}
}

func TestCreatePolicyJSON(t *testing.T) {
	p := policies.Policy{
		Name:    "payments java",
		ID:      "0000000a-0000-0000-0000-00000000000b",
		Version: 3,
		Rules:   nil,
		Outcome: policies.Outcome{
			Inject:         true,
			TracerVersions: map[string]string{"java": "latest"},
		},
	}
	got := createPolicyJSON(p)
	want := `{"name":"payments java","id":"0000000a-0000-0000-0000-00000000000b","version":3,"ddTraceVersions":{"java":"latest"}}`
	if got != want {
		t.Errorf("createPolicyJSON = %s, want %s", got, want)
	}

	// id is omitted when the policy carries no UUID.
	got = createPolicyJSON(policies.Policy{Name: "no id"})
	if want := `{"name":"no id"}`; got != want {
		t.Errorf("createPolicyJSON without id = %s, want %s", got, want)
	}
}

func TestPolicyMatcherDetectsNamespaceLabels(t *testing.T) {
	targets := []Target{{
		Name:              "by-ns-label",
		NamespaceSelector: &NamespaceSelector{MatchLabels: map[string]string{"instrument": "true"}},
	}}
	m := newPolicyMatcher(targets, nil)
	if !m.needsNamespaceLabels {
		t.Errorf("matcher should require namespace labels when a policy reads them")
	}
}
