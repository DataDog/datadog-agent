// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTaskWithAttrs(jobID, bundleID, actionName string) *Task {
	return &Task{
		Data: struct {
			ID         string      `json:"id,omitempty"`
			Type       string      `json:"type,omitempty"`
			Attributes *Attributes `json:"attributes,omitempty"`
		}{
			ID: "t-1",
			Attributes: &Attributes{
				JobId:    jobID,
				BundleID: bundleID,
				Name:     actionName,
			},
		},
	}
}

// TestTask_Validate_Valid verifies that a task with a non-empty JobId passes validation.
func TestTask_Validate_Valid(t *testing.T) {
	task := makeTaskWithAttrs("job-abc", "com.example.bundle", "doThing")
	assert.NoError(t, task.Validate())
}

// TestTask_Validate_NilAttributes verifies that a task with nil Attributes returns an error.
// This is the code path logged as "empty task provided" in workflow_executor.go.
func TestTask_Validate_NilAttributes(t *testing.T) {
	task := &Task{}
	err := task.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty task")
}

// TestTask_Validate_EmptyJobID verifies that a missing JobId is rejected.
// A task without a JobId cannot have its result published, so this is checked early
// and the error is logged before the task reaches execution.
func TestTask_Validate_EmptyJobID(t *testing.T) {
	task := makeTaskWithAttrs("", "com.example.bundle", "doThing")
	err := task.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JobId")
}

// TestTask_GetFQN verifies that the fully-qualified name is "bundleID.actionName".
// This value appears in logs and metrics tags throughout the runner.
func TestTask_GetFQN(t *testing.T) {
	task := makeTaskWithAttrs("job-1", "com.datadoghq.kubernetes.core", "getPods")
	assert.Equal(t, "com.datadoghq.kubernetes.core.getPods", task.GetFQN())
}

// TestExtractInputs_ValidJSON verifies that typed inputs are correctly unmarshaled.
func TestExtractInputs_ValidJSON(t *testing.T) {
	type myInputs struct {
		Namespace string `json:"namespace"`
		Count     int    `json:"count"`
	}
	task := makeTaskWithAttrs("job-1", "b", "a")
	task.Data.Attributes.Inputs = map[string]interface{}{
		"namespace": "kube-system",
		"count":     float64(3), // JSON numbers unmarshal as float64
	}

	inputs, err := ExtractInputs[myInputs](task)

	require.NoError(t, err)
	assert.Equal(t, "kube-system", inputs.Namespace)
	assert.Equal(t, 3, inputs.Count)
}

// TestExtractInputs_UnmarshalError verifies that inputs that cannot be decoded into the
// target type return a descriptive error (which is subsequently logged).
func TestExtractInputs_UnmarshalError(t *testing.T) {
	type strictInputs struct {
		Port int `json:"port"`
	}
	task := makeTaskWithAttrs("job-1", "b", "a")
	// "port" should be a number but is given as a nested object.
	task.Data.Attributes.Inputs = map[string]interface{}{
		"port": map[string]interface{}{"nested": "value"},
	}

	_, err := ExtractInputs[strictInputs](task)

	require.Error(t, err)
}
