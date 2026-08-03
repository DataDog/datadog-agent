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
)

func podWith(ns string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Labels:    labels,
		},
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

	m := newPolicyMatcher(policiesFromTargets(targets), nil)
	for i := range m.policies {
		if nodeUsesNamespaceLabels(m.policies[i].Rules) {
			t.Errorf("matcher should not require namespace labels for pod-label targets")
		}
	}

	out, ok, err := m.Match(podWith("any", map[string]string{"app": "db"}))
	if err != nil {
		t.Fatalf("db pod: unexpected error: %v", err)
	}
	if !ok || out.TracerVersions["java"] != "latest" {
		t.Fatalf("db pod: got %+v ok=%v", out, ok)
	}

	out, ok, err = m.Match(podWith("any", map[string]string{"app": "web"}))
	if err != nil {
		t.Fatalf("web pod: unexpected error: %v", err)
	}
	if !ok || out.TracerVersions["php"] != "latest" {
		t.Fatalf("web pod should hit catch-all: got %+v ok=%v", out, ok)
	}
}

func TestPolicyMatcherReportsUnavailableNamespaceLabels(t *testing.T) {
	targets := []Target{
		{
			Name:              "namespace-label-target",
			NamespaceSelector: &NamespaceSelector{MatchLabels: map[string]string{"instrument": "true"}},
		},
		{Name: "fallback"},
	}
	m := newPolicyMatcher(policiesFromTargets(targets), newMatchTestWmeta(t))

	idx, err := m.matchIndex(podWith("temporarily-unavailable", nil))
	if err == nil {
		t.Fatal("expected unavailable namespace labels to return an error")
	}
	if idx != -1 {
		t.Fatalf("expected no matching index, got %d", idx)
	}
}

func TestPolicyMatcherDetectsNamespaceLabels(t *testing.T) {
	targets := []Target{{
		Name:              "by-ns-label",
		NamespaceSelector: &NamespaceSelector{MatchLabels: map[string]string{"instrument": "true"}},
	}}
	m := newPolicyMatcher(policiesFromTargets(targets), nil)
	if !nodeUsesNamespaceLabels(m.policies[0].Rules) {
		t.Errorf("matcher should require namespace labels when a policy reads them")
	}
}
