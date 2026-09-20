// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Test partitions:
// - source shape: omitted | self (entity kind, no name) | distinct with expressions
// - name expression: literal expression | entity-derived | empty (defaults to entity name)
// - kind: supported | unsupported

// TestResolveSourceGVR covers: known kinds; unsupported kind; apiVersion mismatch.
func TestResolveSourceGVR(t *testing.T) {
	lease, err := resolveSourceGVR(&SourceRef{Kind: "Lease", APIVersion: "coordination.k8s.io/v1"})
	if err != nil {
		t.Fatalf("resolveSourceGVR Lease: %v", err)
	}
	if want := (schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}); lease != want {
		t.Errorf("Lease GVR: got %v, want %v", lease, want)
	}

	if _, err := resolveSourceGVR(&SourceRef{Kind: "VirtualService"}); err == nil {
		t.Errorf("unsupported kind: got no error, want an error")
	}

	if _, err := resolveSourceGVR(&SourceRef{Kind: "Lease", APIVersion: "v1"}); err == nil {
		t.Errorf("apiVersion mismatch: got no error, want an error")
	}
}

// TestResolveSource covers: name from entity label expression; namespace from entity; empty name defaults to entity name.
func TestResolveSource(t *testing.T) {
	evaluator := newTestEvaluator(t)
	rule := &TagRule{Spec: TagRuleSpec{
		Entity: EntityPod,
		Source: &SourceRef{
			Kind:      "Lease",
			Namespace: "entity.metadata.namespace",
			Name:      "entity.metadata.labels['app.kubernetes.io/name'] + '-leader-election'",
		},
	}}
	pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", map[string]string{"app.kubernetes.io/name": "my-controller"})

	coords, distinct, err := resolveSource(rule, pod, evaluator)
	if err != nil {
		t.Fatalf("resolveSource: %v", err)
	}
	if !distinct {
		t.Fatalf("distinct: got false, want true")
	}
	if coords.namespace != "my-namespace" {
		t.Errorf("namespace: got %q, want %q", coords.namespace, "my-namespace")
	}
	if coords.name != "my-controller-leader-election" {
		t.Errorf("name: got %q, want %q", coords.name, "my-controller-leader-election")
	}

	// Self source: no source, and source kind == entity kind without a name.
	selfRule := &TagRule{Spec: TagRuleSpec{Entity: EntityNode, Tag: "is_schedulable", Value: Value{Type: ValueBool}}}
	if _, distinct, err := resolveSource(selfRule, newEntity(EntityNode, "node-a", "", nil), evaluator); err != nil || distinct {
		t.Errorf("omitted source: got distinct=%v err=%v, want distinct=false err=nil", distinct, err)
	}
}
