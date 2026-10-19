// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodeunschedulable

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestEvaluate(t *testing.T) {
	rules, err := Compile(
		[]LabelRule{
			{Key: "example.com/drain", Value: "true"},
			{Key: "nvidia.com/mig.config.state", Value: "failed"},
			{Key: "nvidia.com/mig.config.state", Value: "rebooting"},
			{Key: "example.com/drain", Value: true}, // the same rule again, as a YAML bool
		},
		[]ConditionRule{{Type: "Maintenance", Status: "True"}},
	)
	require.NoError(t, err)
	assert.True(t, rules.HasLabelRules())

	labels := map[string]string{"example.com/drain": "True", "nvidia.com/mig.config.state": "failed", "kubernetes.io/hostname": "n1"}
	conditions := []Condition{{Type: "Ready", Status: "True"}, {Type: "Maintenance", Status: "True"}}
	assert.Equal(t, []string{
		"cordoned",
		"example.com/drain:true",
		"maintenance:true",
		"nvidia.com/mig.config.state:failed",
	}, rules.Evaluate(labels, conditions, true))

	// A label value or a condition status that is not listed does not match.
	labels["nvidia.com/mig.config.state"] = "success"
	conditions[1].Status = "False"
	assert.Equal(t, []string{"example.com/drain:true"}, rules.Evaluate(labels, conditions, false))

	// Nothing matches: an empty list, never nil.
	got := rules.Evaluate(nil, nil, false)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

// Label keys are case-sensitive like in Kubernetes; values are not.
func TestEvaluateLabelKeyIsCaseSensitive(t *testing.T) {
	rules, err := Compile([]LabelRule{{Key: "example.com/drain", Value: "true"}}, nil)
	require.NoError(t, err)
	assert.Empty(t, rules.Evaluate(map[string]string{"example.com/Drain": "true"}, nil, false))
	assert.Equal(t, []string{"example.com/drain:true"}, rules.Evaluate(map[string]string{"example.com/drain": "TRUE"}, nil, false))

	// A key with capitals matches only itself and is reported lowercased.
	rules, err = Compile([]LabelRule{{Key: "example.com/Drain", Value: "true"}}, nil)
	require.NoError(t, err)
	assert.Empty(t, rules.Evaluate(map[string]string{"example.com/drain": "true"}, nil, false))
	assert.Equal(t, []string{"example.com/drain:true"}, rules.Evaluate(map[string]string{"example.com/Drain": "true"}, nil, false))
}

// Cordon is always checked, even with no rules.
func TestEvaluateCordonWithoutRules(t *testing.T) {
	rules, err := Compile(nil, nil)
	require.NoError(t, err)
	assert.False(t, rules.HasLabelRules(), "no label rules: the labels need not be read")
	assert.Equal(t, []string{"cordoned"}, rules.Evaluate(map[string]string{"example.com/drain": "true"}, nil, true))
	assert.Empty(t, rules.Evaluate(map[string]string{"example.com/drain": "true"}, nil, false))
}

func TestCompileErrors(t *testing.T) {
	tests := map[string]struct {
		labels     []LabelRule
		conditions []ConditionRule
	}{
		"label without key":                 {labels: []LabelRule{{Value: "true"}}},
		"label without value":               {labels: []LabelRule{{Key: "example.com/drain"}}},
		"label with an empty value":         {labels: []LabelRule{{Key: "example.com/drain", Value: ""}}},
		"label with a number value":         {labels: []LabelRule{{Key: "example.com/drain", Value: 1}}},
		"label with a space":                {labels: []LabelRule{{Key: "example drain", Value: "true"}}},
		"label value with a slash":          {labels: []LabelRule{{Key: "example.com/drain", Value: "a/b"}}},
		"reason too long":                   {labels: []LabelRule{{Key: strings.Repeat("a", maxReasonLen), Value: "true"}}},
		"condition without type":            {conditions: []ConditionRule{{Status: "True"}}},
		"condition without status":          {conditions: []ConditionRule{{Type: "Maintenance"}}},
		"condition with a bad status":       {conditions: []ConditionRule{{Type: "Maintenance", Status: "maybe"}}},
		"condition with a lowercase status": {conditions: []ConditionRule{{Type: "Maintenance", Status: "true"}}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Compile(tt.labels, tt.conditions)
			assert.Error(t, err)
		})
	}
}

func TestLongestReasonFitsTheTagLimit(t *testing.T) {
	key := strings.Repeat("a", maxReasonLen-len(":true"))
	rules, err := Compile([]LabelRule{{Key: key, Value: "true"}}, nil)
	require.NoError(t, err)
	reasons := rules.Evaluate(map[string]string{key: "true"}, nil, false)
	require.Len(t, reasons, 1)
	assert.Len(t, TagKey+":"+reasons[0], 200)
}

// Reasons are normalized like tags; matching still uses the exact label value.
func TestEvaluateCanonicalReasons(t *testing.T) {
	rules, err := Compile([]LabelRule{
		{Key: "example.com/state", Value: "a__b"},
		{Key: "example.com/state", Value: "a_b"},
		{Key: "example.com/phase", Value: "test_"},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com/state:a_b"}, rules.Evaluate(map[string]string{"example.com/state": "a__b"}, nil, false))
	assert.Equal(t, []string{"example.com/state:a_b"}, rules.Evaluate(map[string]string{"example.com/state": "a_b"}, nil, false))
	assert.Empty(t, rules.Evaluate(map[string]string{"example.com/state": "a___c"}, nil, false))
	assert.Equal(t, []string{"example.com/phase:test"}, rules.Evaluate(map[string]string{"example.com/phase": "test_"}, nil, false))
	assert.Empty(t, rules.Evaluate(map[string]string{"example.com/phase": "test"}, nil, false), "matching uses the value as is")

	// A value that is nothing once canonical is rejected.
	_, err = Compile([]LabelRule{{Key: "example.com/state", Value: "__"}}, nil)
	assert.Error(t, err)
}

// DD_GPU_UNSCHEDULABLE_NODES_LABELS and _CONDITIONS hold JSON lists.
func TestLoadFromEnv(t *testing.T) {
	t.Setenv("DD_GPU_UNSCHEDULABLE_NODES_LABELS", `[{"key":"example.com/drain","value":"true"},{"key":"nvidia.com/mig.config.state","value":"failed"}]`)
	t.Setenv("DD_GPU_UNSCHEDULABLE_NODES_CONDITIONS", `[{"type":"Maintenance","status":"True"}]`)
	rules, err := Load(configmock.New(t))
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com/drain:true", "maintenance:true"},
		rules.Evaluate(map[string]string{"example.com/drain": "true"}, []Condition{{Type: "Maintenance", Status: "True"}}, false))
}

// JSON booleans work in the env like unquoted YAML booleans in the file.
func TestLoadFromEnvJSONBooleans(t *testing.T) {
	t.Setenv("DD_GPU_UNSCHEDULABLE_NODES_LABELS", `[{"key":"example.com/drain","value":true}]`)
	t.Setenv("DD_GPU_UNSCHEDULABLE_NODES_CONDITIONS", `[{"type":"Maintenance","status":true}]`)
	rules, err := Load(configmock.New(t))
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com/drain:true", "maintenance:true"},
		rules.Evaluate(map[string]string{"example.com/drain": "true"}, []Condition{{Type: "Maintenance", Status: "True"}}, false))
}

// Malformed JSON in the env is invalid rules, not an empty rule set; an empty list is fine.
func TestLoadFromEnvMalformedJSON(t *testing.T) {
	t.Setenv("DD_GPU_UNSCHEDULABLE_NODES_LABELS", `[{"key":"example.com/drain"`)
	_, err := Load(configmock.New(t))
	assert.Error(t, err)
}

func TestLoadFromEnvEmptyList(t *testing.T) {
	t.Setenv("DD_GPU_UNSCHEDULABLE_NODES_CONDITIONS", `[]`)
	_, err := Load(configmock.New(t))
	assert.NoError(t, err)
}

func TestLoadFromYAML(t *testing.T) {
	rules, err := Load(configmock.NewFromYAML(t, `
gpu:
  unschedulable_nodes:
    labels:
      - key: example.com/drain
        value: true
    conditions:
      - type: Maintenance
        status: True
`))
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com/drain:true", "maintenance:true"},
		rules.Evaluate(map[string]string{"example.com/drain": "true"}, []Condition{{Type: "Maintenance", Status: "True"}}, false))
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	_, err := Load(configmock.NewFromYAML(t, `
gpu:
  unschedulable_nodes:
    labels:
      - key: example.com/drain
        values: [true]
`))
	assert.Error(t, err)
}
