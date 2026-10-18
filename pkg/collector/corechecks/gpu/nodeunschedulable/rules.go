// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodeunschedulable decides whether a GPU node is unschedulable (cordoned, or
// matching a gpu.unschedulable_nodes rule) and turns it into gpu.device.unavailable series.
package nodeunschedulable

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

const (
	// LabelsConfigKey holds the label rules: a list of {key, value}.
	LabelsConfigKey = "gpu.unschedulable_nodes.labels"
	// ConditionsConfigKey holds the condition rules: a list of {type, status}.
	ConditionsConfigKey = "gpu.unschedulable_nodes.conditions"

	// CordonReason is the reason of a cordoned node, always checked.
	CordonReason = "cordoned"

	// maxReasonLen keeps the whole tag, unschedulable_reason:<reason>, within the
	// 200-character tag limit.
	maxReasonLen = 200 - len(TagKey+":")
)

var (
	// validPart is what a label key, a label value or a condition type and status may
	// contain once lowercased, so that the reason is a valid tag value as is.
	validPart = regexp.MustCompile(`^[a-z0-9._/-]+$`)
	// validReason is a reason once canonical: <key>:<value>, both non-empty.
	validReason = regexp.MustCompile(`^[a-z0-9._/-]+:[a-z0-9._-]+$`)
	underscores = regexp.MustCompile(`_{2,}`)
)

// canonical returns a reason the way the backend normalizes tags (repeated underscores
// collapsed, trailing ones removed), so two reasons never collide on one series.
func canonical(reason string) string {
	return strings.TrimRight(underscores.ReplaceAllString(reason, "_"), "_")
}

// LabelRule matches a node label with the given value. The value may be a string or
// a bool (an unquoted YAML true) and is compared case-insensitively.
type LabelRule struct {
	Key   string `mapstructure:"key"`
	Value any    `mapstructure:"value"`
}

// ConditionRule matches a node condition with the given status (True, False or
// Unknown, case-insensitive). The type is compared case-insensitively.
type ConditionRule struct {
	Type   string `mapstructure:"type"`
	Status any    `mapstructure:"status"`
}

// Condition is a node condition reduced to type and status.
type Condition struct {
	Type   string
	Status string
}

// Rules is a validated set of rules.
type Rules struct {
	// labels maps a label key, case-sensitive like in Kubernetes, to its lowercased values.
	labels map[string][]string
	// conditions maps a lowercased condition type to its lowercased statuses.
	conditions map[string][]string
}

// Load reads and validates the rules from the configuration.
func Load(cfg model.Reader) (*Rules, error) {
	for _, key := range []string{LabelsConfigKey, ConditionsConfigKey} {
		// The env parser only logs a JSON syntax error and leaves the value nil.
		if v := reflect.ValueOf(cfg.Get(key)); cfg.GetSource(key) == model.SourceEnvVar && (!v.IsValid() || v.Kind() == reflect.Slice && v.IsNil()) {
			return nil, fmt.Errorf("%s: the environment variable is not a valid JSON list", key)
		}
	}
	var labels []LabelRule
	var conditions []ConditionRule
	if err := structure.UnmarshalKey(cfg, LabelsConfigKey, &labels, structure.ErrorUnused); err != nil {
		return nil, fmt.Errorf("%s: %w", LabelsConfigKey, err)
	}
	if err := structure.UnmarshalKey(cfg, ConditionsConfigKey, &conditions, structure.ErrorUnused); err != nil {
		return nil, fmt.Errorf("%s: %w", ConditionsConfigKey, err)
	}
	return Compile(labels, conditions)
}

// Compile validates the rules. The same key with several values matches any of them.
func Compile(labels []LabelRule, conditions []ConditionRule) (*Rules, error) {
	r := &Rules{labels: map[string][]string{}, conditions: map[string][]string{}}
	var errs []error
	// add records value under key; the reason it reports is checked with the key lowercased.
	add := func(m map[string][]string, where, key, value string) {
		name := strings.ToLower(key)
		reason := name + ":" + value
		switch {
		case !validPart.MatchString(name) || !validPart.MatchString(value) || strings.Contains(value, "/") || !validReason.MatchString(canonical(reason)):
			errs = append(errs, fmt.Errorf("%s: %q is not a valid label or condition", where, reason))
		case len(reason) > maxReasonLen:
			errs = append(errs, fmt.Errorf("%s: %q is longer than %d characters", where, reason, maxReasonLen))
		case !slices.Contains(m[key], value):
			m[key] = append(m[key], value)
		}
	}

	for i, l := range labels {
		where := fmt.Sprintf("%s[%d]", LabelsConfigKey, i)
		value, err := operand(l.Value)
		if l.Key == "" || err != nil {
			errs = append(errs, fmt.Errorf("%s: key and value are required (value: %v)", where, err))
			continue
		}
		add(r.labels, where, l.Key, value)
	}
	for i, c := range conditions {
		where := fmt.Sprintf("%s[%d]", ConditionsConfigKey, i)
		status, err := operand(c.Status)
		if c.Type == "" || err != nil {
			errs = append(errs, fmt.Errorf("%s: type and status are required (status: %v)", where, err))
			continue
		}
		if status != "true" && status != "false" && status != "unknown" {
			errs = append(errs, fmt.Errorf("%s: status must be True, False or Unknown, got %q", where, status))
			continue
		}
		add(r.conditions, where, strings.ToLower(c.Type), status)
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return r, nil
}

// HasLabelRules tells whether the node labels are needed to evaluate the rules.
func (r *Rules) HasLabelRules() bool {
	return len(r.labels) > 0
}

// operand converts a configured value to its lowercase string form: a non-empty
// string or a bool.
func operand(v any) (string, error) {
	switch t := v.(type) {
	case string:
		if t == "" {
			return "", errors.New("empty")
		}
		return strings.ToLower(t), nil
	case bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case nil:
		return "", errors.New("missing")
	default:
		return "", fmt.Errorf("must be a string or a bool, got %T (%v)", v, v)
	}
}

// Evaluate returns the sorted reasons (cordoned, <key>:<value>, <type>:<status>) that
// make the node unschedulable; an empty list means it is schedulable.
func (r *Rules) Evaluate(labels map[string]string, conditions []Condition, unschedulable bool) []string {
	reasons := []string{}
	if unschedulable {
		reasons = append(reasons, CordonReason)
	}
	for k, v := range labels {
		v = strings.ToLower(v)
		if slices.Contains(r.labels[k], v) {
			reasons = append(reasons, canonical(strings.ToLower(k)+":"+v))
		}
	}
	for _, c := range conditions {
		t, s := strings.ToLower(c.Type), strings.ToLower(c.Status)
		if slices.Contains(r.conditions[t], s) {
			reasons = append(reasons, canonical(t+":"+s))
		}
	}
	slices.Sort(reasons)
	return slices.Compact(reasons)
}
