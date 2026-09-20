// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Test partitions:
// - selector part: empty | matchLabels | matchExpressions In | NotIn | Exists | DoesNotExist
// - namespace scope: unset | matching | non-matching (pod rules)
// - entity kind: pod (namespace honored) | node (namespace ignored)

func newEntity(kind EntityKind, name, namespace string, entityLabels map[string]string) *unstructured.Unstructured {
	metadata := map[string]any{"name": name}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	if entityLabels != nil {
		labelsAny := make(map[string]any, len(entityLabels))
		for key, value := range entityLabels {
			labelsAny[key] = value
		}
		metadata["labels"] = labelsAny
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       string(kind),
		"metadata":   metadata,
		"spec":       map[string]any{},
	}}
}

// setAnnotations sets metadata.annotations on an unstructured object.
func setAnnotations(u *unstructured.Unstructured, annotations map[string]string) {
	annotationsAny := make(map[string]any, len(annotations))
	for key, value := range annotations {
		annotationsAny[key] = value
	}
	u.Object["metadata"].(map[string]any)["annotations"] = annotationsAny
}

// setContainers sets spec.containers with the given names.
func setContainers(u *unstructured.Unstructured, names ...string) {
	containers := make([]any, 0, len(names))
	for _, name := range names {
		containers = append(containers, map[string]any{"name": name})
	}
	u.Object["spec"].(map[string]any)["containers"] = containers
}

// TestSelectorMatchLabels covers: empty selector matches all; matchLabels match; matchLabels mismatch.
func TestSelectorMatchLabels(t *testing.T) {
	entity := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", map[string]string{"app": "my-controller"})

	for _, tt := range []struct {
		name     string
		selector Selector
		want     bool
	}{
		{"empty selector matches all", Selector{}, true},
		{"labels match", Selector{MatchLabels: map[string]string{"app": "my-controller"}}, true},
		{"labels mismatch", Selector{MatchLabels: map[string]string{"app": "other"}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.selector.matches(entity, EntityPod)
			if err != nil {
				t.Fatalf("matches: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSelectorMatchExpressions covers: In match/mismatch; NotIn; Exists; DoesNotExist.
func TestSelectorMatchExpressions(t *testing.T) {
	entity := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", map[string]string{"app": "my-controller", "tier": "web"})

	for _, tt := range []struct {
		name    string
		require metav1.LabelSelectorRequirement
		want    bool
	}{
		{"In match", metav1.LabelSelectorRequirement{Key: "tier", Operator: "In", Values: []string{"web", "api"}}, true},
		{"In mismatch", metav1.LabelSelectorRequirement{Key: "tier", Operator: "In", Values: []string{"db"}}, false},
		{"NotIn", metav1.LabelSelectorRequirement{Key: "tier", Operator: "NotIn", Values: []string{"db"}}, true},
		{"Exists", metav1.LabelSelectorRequirement{Key: "app", Operator: "Exists"}, true},
		{"DoesNotExist", metav1.LabelSelectorRequirement{Key: "zone", Operator: "DoesNotExist"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			selector := Selector{MatchExpressions: []metav1.LabelSelectorRequirement{tt.require}}
			got, err := selector.matches(entity, EntityPod)
			if err != nil {
				t.Fatalf("matches: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSelectorNamespace covers: unset namespace; matching namespace; non-matching namespace (pods); node ignores namespace.
func TestSelectorNamespace(t *testing.T) {
	pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", nil)
	node := newEntity(EntityNode, "node-a", "", nil)

	selector := Selector{Namespace: "my-namespace"}
	if got, _ := mustMatch(t, selector, pod, EntityPod); !got {
		t.Errorf("matching namespace: got false, want true")
	}

	selector = Selector{Namespace: "other-namespace"}
	if got, _ := mustMatch(t, selector, pod, EntityPod); got {
		t.Errorf("non-matching namespace: got true, want false")
	}

	// Nodes are cluster-scoped: the namespace is ignored.
	selector = Selector{Namespace: "my-namespace"}
	if got, _ := mustMatch(t, selector, node, EntityNode); !got {
		t.Errorf("node with namespace selector: got false, want true")
	}
}

func mustMatch(t *testing.T, selector Selector, entity *unstructured.Unstructured, kind EntityKind) (bool, error) {
	t.Helper()
	return selector.matches(entity, kind)
}

// TestValidateRule covers: valid bool; valid string set; missing expression; default outside values; node with namespace; bad entity.
func TestValidateRule(t *testing.T) {
	for _, tt := range []struct {
		name    string
		rule    TagRule
		wantErr bool
	}{
		{
			"valid bool",
			TagRule{Spec: TagRuleSpec{Entity: EntityPod, Tag: "is_leader", Value: Value{Type: ValueBool, Bool: &BoolValue{Expression: "true"}}}},
			false,
		},
		{
			"valid string set",
			TagRule{Spec: TagRuleSpec{Entity: EntityPod, Tag: "owning_team", Value: Value{Type: ValueStringSet, StringSet: &StringSetValue{Values: []string{"frontend"}, Expression: "'frontend'", OnError: OnErrorDefault, Default: "frontend"}}}},
			false,
		},
		{
			"bool without expression",
			TagRule{Spec: TagRuleSpec{Entity: EntityPod, Tag: "t", Value: Value{Type: ValueBool}}},
			true,
		},
		{
			"default outside values",
			TagRule{Spec: TagRuleSpec{Entity: EntityPod, Tag: "t", Value: Value{Type: ValueStringSet, StringSet: &StringSetValue{Values: []string{"frontend"}, Expression: "'x'", OnError: OnErrorDefault, Default: "unknown"}}}},
			true,
		},
		{
			"node with namespace selector",
			TagRule{Spec: TagRuleSpec{Entity: EntityNode, Selector: Selector{Namespace: "ns"}, Tag: "t", Value: Value{Type: ValueBool, Bool: &BoolValue{Expression: "true"}}}},
			true,
		},
		{
			"unsupported entity",
			TagRule{Spec: TagRuleSpec{Entity: EntityKind("service"), Tag: "t", Value: Value{Type: ValueBool, Bool: &BoolValue{Expression: "true"}}}},
			true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.validateRule()
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRule: got err %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
