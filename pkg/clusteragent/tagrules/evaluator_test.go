// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"testing"
)

// Test partitions:
// - expression kind: bool field-derived | bool source comparison | string with has() fallback
// - failure mode: compile error | wrong result type | runtime error | missing key
// - value domain: present key | absent key

func newTestEvaluator(t *testing.T) *Evaluator {
	t.Helper()
	evaluator, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	return evaluator
}

func podMap(labels map[string]any) map[string]any {
	return map[string]any{
		"metadata": map[string]any{
			"name":      "ben-bitdiddle",
			"namespace": "my-namespace",
			"labels":    labels,
		},
		"spec": map[string]any{},
	}
}

func leaseMap(holder string) map[string]any {
	return map[string]any{
		"metadata": map[string]any{"name": "leader-election", "namespace": "my-namespace"},
		"spec":     map[string]any{"holderIdentity": holder},
	}
}

// TestEvalBool covers: bool field-derived + present key; bool source comparison + holder match; holder mismatch.
func TestEvalBool(t *testing.T) {
	evaluator := newTestEvaluator(t)

	node := map[string]any{"metadata": map[string]any{"name": "node-a"}, "spec": map[string]any{"unschedulable": true}}
	got, err := evaluator.EvalBool("!has(entity.spec.unschedulable) || !entity.spec.unschedulable", node, node)
	if err != nil {
		t.Fatalf("EvalBool unschedulable: %v", err)
	}
	if got {
		t.Errorf("cordon expression: got true, want false")
	}

	pod := podMap(nil)
	got, err = evaluator.EvalBool("source.spec.holderIdentity == entity.metadata.name", pod, leaseMap("ben-bitdiddle"))
	if err != nil {
		t.Fatalf("EvalBool holder match: %v", err)
	}
	if !got {
		t.Errorf("holder match: got false, want true")
	}

	got, err = evaluator.EvalBool("source.spec.holderIdentity == entity.metadata.name", pod, leaseMap("eva-lu-ator"))
	if err != nil {
		t.Fatalf("EvalBool holder mismatch: %v", err)
	}
	if got {
		t.Errorf("holder mismatch: got true, want false")
	}
}

// TestEvalString covers: has() fallback with pod key present; fallback to source; both keys absent (runtime error).
func TestEvalString(t *testing.T) {
	evaluator := newTestEvaluator(t)

	expression := `('owner' in entity.metadata.labels) ? entity.metadata.labels['owner'] : source.metadata.labels['owner']`

	pod := podMap(map[string]any{"owner": "frontend"})
	namespace := map[string]any{"metadata": map[string]any{"name": "my-namespace", "labels": map[string]any{"owner": "infra"}}}

	got, err := evaluator.EvalString(expression, pod, namespace)
	if err != nil {
		t.Fatalf("EvalString pod label present: %v", err)
	}
	if got != "frontend" {
		t.Errorf("pod label present: got %q, want %q", got, "frontend")
	}

	got, err = evaluator.EvalString(expression, podMap(nil), namespace)
	if err != nil {
		t.Fatalf("EvalString namespace fallback: %v", err)
	}
	if got != "infra" {
		t.Errorf("namespace fallback: got %q, want %q", got, "infra")
	}

	if _, err := evaluator.EvalString(expression, podMap(nil), map[string]any{"metadata": map[string]any{}}); err == nil {
		t.Errorf("both labels absent: got no error, want a runtime error")
	}
}

// TestEvalCompileErrors covers: non-bool expression for EvalBool; non-string for EvalString; syntax error.
func TestEvalCompileErrors(t *testing.T) {
	evaluator := newTestEvaluator(t)

	if _, err := evaluator.EvalBool("'a string'", podMap(nil), nil); err == nil {
		t.Errorf("string expression as bool: got no error, want a type error")
	}
	if _, err := evaluator.EvalString("true", podMap(nil), nil); err == nil {
		t.Errorf("bool expression as string: got no error, want a type error")
	}
	if _, err := evaluator.EvalBool("entity.metadata.", podMap(nil), nil); err == nil {
		t.Errorf("syntax error: got no error, want a compile error")
	}
}
