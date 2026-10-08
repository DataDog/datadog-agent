// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package types

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestParseConfigAgentTask(t *testing.T) {
	t.Run("string args", func(t *testing.T) {
		task, err := ParseConfigAgentTask([]byte(`{"task_type":"flare","uuid":"a_uuid","args":{"case_id":"123","user_handle":"a@b.c"}}`), state.Metadata{})
		require.NoError(t, err)
		assert.Equal(t, "flare", task.Config.TaskType)
		assert.Equal(t, "a_uuid", task.Config.UUID)
		assert.Equal(t, map[string]string{"case_id": "123", "user_handle": "a@b.c"}, task.Config.TaskArgs)
		assert.Len(t, task.Config.RawTaskArgs, 2)
	})

	t.Run("mixed args", func(t *testing.T) {
		task, err := ParseConfigAgentTask([]byte(`{"task_type":"trigger_payloads","uuid":"a_uuid","args":{"source":"fleet","payloads":["agent-health"],"nested":{"a":1}}}`), state.Metadata{})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"source": "fleet"}, task.Config.TaskArgs)

		var payloads []string
		require.NoError(t, json.Unmarshal(task.Config.RawTaskArgs["payloads"], &payloads))
		assert.Equal(t, []string{"agent-health"}, payloads)
		assert.JSONEq(t, `{"a":1}`, string(task.Config.RawTaskArgs["nested"]))
	})

	t.Run("no args", func(t *testing.T) {
		task, err := ParseConfigAgentTask([]byte(`{"task_type":"trigger_payloads","uuid":"a_uuid"}`), state.Metadata{})
		require.NoError(t, err)
		assert.Nil(t, task.Config.TaskArgs)
		assert.Nil(t, task.Config.RawTaskArgs)
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := ParseConfigAgentTask([]byte(`{"task_type":1}`), state.Metadata{})
		assert.Error(t, err)
	})
}

func TestPartialFailureError(t *testing.T) {
	inner := errors.New("boom")
	err := NewPartialFailureError(inner)

	var partialErr *PartialFailureError
	assert.True(t, errors.As(err, &partialErr))
	assert.ErrorIs(t, err, inner)
	assert.Equal(t, "partial failure: boom", err.Error())
	assert.Equal(t, "partial failure", NewPartialFailureError(nil).Error())
}
