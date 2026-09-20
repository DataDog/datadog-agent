// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// GroupName is the API group of the TagRule CRD: a dedicated group owned
	// by the Agent, kept separate from the datadog-operator group.
	GroupName = "agent.datadoghq.com"
	// GroupVersion is the served version of the TagRule CRD.
	GroupVersion = "v1alpha1"
	// TagRuleKind is the CRD kind.
	TagRuleKind = "TagRule"
	// TagRuleResource is the plural resource name of the TagRule CRD.
	TagRuleResource = "tagrules"

	// ADContainerTagsAnnotationFormat is the per-container tags annotation the
	// tagger reads unconditionally. It carries a JSON map of tag key to value.
	ADContainerTagsAnnotationFormat = "ad.datadoghq.com/%s.tags"
	// ManagedTagKeysAnnotation is the ownership ledger on an entity: a
	// comma-separated list of tag keys currently owned by tag rules.
	ManagedTagKeysAnnotation = "datadoghq.com/managed-tag-keys"
	// NodeTagAnnotationPrefix is the reserved annotation prefix for
	// rule-owned tags on nodes. The prefix itself is the ownership marker.
	NodeTagAnnotationPrefix = "tags.datadoghq.com/tag."
	// CleanupFinalizer is added to every TagRule and removed only after the
	// deletion sweep has stripped the rule's tag key from matching entities.
	CleanupFinalizer = "datadoghq.com/tag-rule-cleanup"

	// DefaultFlipHold is the minimum time a tag value must hold before a flip
	// is written. Matches the fleet-wide 15s cadence.
	DefaultFlipHold = 15

	adTagsContainerKey = "spec.containers"
	adTagsInitKey      = "spec.initContainers"
)

// EntityKind is the kind of Kubernetes object a rule targets.
type EntityKind string

// Supported entity kinds in v1.
const (
	EntityPod  EntityKind = "pod"
	EntityNode EntityKind = "node"
)

// ValueKind selects how a rule's value is computed.
type ValueKind string

// Supported value kinds in v1.
const (
	ValueBool      ValueKind = "bool"
	ValueStringSet ValueKind = "string_set"
)

// OnErrorAction selects what happens when a string-set expression errors or
// produces a value outside the declared set.
type OnErrorAction string

// Supported error actions.
const (
	OnErrorDrop    OnErrorAction = "drop"
	OnErrorDefault OnErrorAction = "default"
)

// Selector scopes a rule to matching entities. An empty selector matches every
// entity of the kind. Namespace applies to pods only.
type Selector struct {
	Namespace        string                            `json:"namespace,omitempty"`
	MatchLabels      map[string]string                 `json:"matchLabels,omitempty"`
	MatchExpressions []metav1.LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// SourceRef describes the second object made available to value expressions
// as `source`. Kind and APIVersion resolve to a resource; Name and Namespace
// are CEL string expressions evaluated against the entity. Empty expressions
// default to the entity's own name and, for namespaced resources, namespace.
type SourceRef struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
}

// BoolValue is a CEL expression over entity/source evaluating to a bool.
type BoolValue struct {
	Expression string `json:"expression"`
}

// StringSetValue is a CEL expression over entity/source evaluating to a
// string, constrained to the declared Values domain.
type StringSetValue struct {
	Values     []string      `json:"values"`
	Expression string        `json:"expression"`
	OnError    OnErrorAction `json:"onError,omitempty"`
	Default    string        `json:"default,omitempty"`
}

// Value is the rule's value definition. Exactly one of Bool or StringSet is
// set, matching Type.
type Value struct {
	Type      ValueKind       `json:"type"`
	Bool      *BoolValue      `json:"bool,omitempty"`
	StringSet *StringSetValue `json:"stringSet,omitempty"`
}

// TagRuleSpec is the desired state of a TagRule.
type TagRuleSpec struct {
	Entity          EntityKind `json:"entity"`
	Selector        Selector   `json:"selector,omitempty"`
	Tag             string     `json:"tag"`
	Source          *SourceRef `json:"source,omitempty"`
	Value           Value      `json:"value"`
	FlipHoldSeconds *int64     `json:"flipHoldSeconds,omitempty"`
}

// RuleState is the aggregated rule health.
type RuleState string

// Rule states.
const (
	RuleStateActive      RuleState = "active"
	RuleStateError       RuleState = "error"
	RuleStateQuarantined RuleState = "quarantined"
)

// EntitiesStatus counts entities touched by the rule.
type EntitiesStatus struct {
	Matched int `json:"matched,omitempty"`
	Tagged  int `json:"tagged,omitempty"`
}

// ValuesStatus is the runtime view of a string-set rule's value domain.
type ValuesStatus struct {
	Observed map[string]int `json:"observed,omitempty"`
	Dropped  int            `json:"dropped,omitempty"`
}

// TagRuleStatus is the observed state of a TagRule.
type TagRuleStatus struct {
	State              RuleState          `json:"state,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	LastEvaluated      *metav1.Time       `json:"lastEvaluated,omitempty"`
	Entities           EntitiesStatus     `json:"entities,omitempty"`
	Values             ValuesStatus       `json:"values,omitempty"`
	ObservedSelector   *Selector          `json:"observedSelector,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// TagRule is a single tag rule. One rule per CR: the CR name is the rule's
// identity, spec.tag is globally unique across the cluster.
type TagRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TagRuleSpec   `json:"spec,omitempty"`
	Status            TagRuleStatus `json:"status,omitempty"`
}

// flipHold returns the rule's flip hold in seconds.
func (s *TagRuleSpec) flipHold() int64 {
	if s.FlipHoldSeconds != nil && *s.FlipHoldSeconds > 0 {
		return *s.FlipHoldSeconds
	}
	return DefaultFlipHold
}

// ruleValidationError marks configuration the controller refuses to write with,
// reported through the rule status instead of a crash.
type ruleValidationError struct{ msg string }

func (e *ruleValidationError) Error() string { return e.msg }

func ruleValidationErrorf(format string, args ...any) error {
	return &ruleValidationError{msg: fmt.Sprintf(format, args...)}
}

// validateRule checks the invariants the admission webhook also enforces, so
// the controller fails closed on rules that bypassed validation.
func (r *TagRule) validateRule() error {
	switch r.Spec.Entity {
	case EntityPod, EntityNode:
	default:
		return ruleValidationErrorf("unsupported entity kind %q", r.Spec.Entity)
	}
	if r.Spec.Entity == EntityNode && r.Spec.Selector.Namespace != "" {
		return ruleValidationErrorf("namespace selector is not supported for node rules")
	}
	if r.Spec.Tag == "" {
		return ruleValidationErrorf("spec.tag must not be empty")
	}
	switch r.Spec.Value.Type {
	case ValueBool:
		if r.Spec.Value.Bool == nil || r.Spec.Value.Bool.Expression == "" {
			return ruleValidationErrorf("type bool requires value.bool.expression")
		}
		if r.Spec.Value.StringSet != nil {
			return ruleValidationErrorf("type bool must not set value.stringSet")
		}
	case ValueStringSet:
		ss := r.Spec.Value.StringSet
		if ss == nil || len(ss.Values) == 0 || ss.Expression == "" {
			return ruleValidationErrorf("type string_set requires value.stringSet.values and value.stringSet.expression")
		}
		if r.Spec.Value.Bool != nil {
			return ruleValidationErrorf("type string_set must not set value.bool")
		}
		if ss.OnError == OnErrorDefault {
			if !containsString(ss.Values, ss.Default) {
				return ruleValidationErrorf("value.stringSet.default %q must be a member of values", ss.Default)
			}
		}
	default:
		return ruleValidationErrorf("unsupported value type %q", r.Spec.Value.Type)
	}
	return nil
}

// tagRuleFromUnstructured converts a TagRule CR into its typed form.
func tagRuleFromUnstructured(u *unstructured.Unstructured) (*TagRule, error) {
	var rule TagRule
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &rule); err != nil {
		return nil, fmt.Errorf("converting TagRule %s: %w", u.GetName(), err)
	}
	return &rule, nil
}

// toUnstructured converts a typed TagRule back into its unstructured form.
func (r *TagRule) toUnstructured() (*unstructured.Unstructured, error) {
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(r)
	if err != nil {
		return nil, fmt.Errorf("converting TagRule %s to unstructured: %w", r.Name, err)
	}
	return &unstructured.Unstructured{Object: obj}, nil
}

// parseManagedTagKeys decodes the ownership ledger annotation.
func parseManagedTagKeys(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		if key := strings.TrimSpace(part); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// formatManagedTagKeys encodes the ownership ledger annotation.
func formatManagedTagKeys(keys []string) string {
	return strings.Join(keys, ",")
}

// containerNames returns every container name on a pod that can carry the AD
// tags annotation: regular and init containers.
func containerNames(pod *unstructured.Unstructured) ([]string, error) {
	var names []string
	for _, field := range []string{adTagsContainerKey, adTagsInitKey} {
		raw, found, err := unstructured.NestedSlice(pod.Object, strings.Split(field, ".")...)
		if err != nil {
			return nil, fmt.Errorf("reading %s of pod %s: %w", field, pod.GetName(), err)
		}
		if !found {
			continue
		}
		for _, item := range raw {
			container, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("unexpected non-map entry in %s of pod %s", field, pod.GetName())
			}
			name, ok := container["name"].(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("container without name in %s of pod %s", field, pod.GetName())
			}
			names = append(names, name)
		}
	}
	return names, nil
}

// entityValueString renders a computed value for annotations: bools as
// true/false, strings as-is.
func entityValueString(value bool) string {
	return strconv.FormatBool(value)
}
