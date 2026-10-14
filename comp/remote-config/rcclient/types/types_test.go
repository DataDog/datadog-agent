// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package types

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestParseConfigAgentTask(t *testing.T) {
	t.Run("string args", func(t *testing.T) {
		task, err := ParseConfigAgentTask([]byte(`{"task_type":"trigger_payloads","uuid":"a_uuid","args":{"payloads":"inventory-agent,agent-health"}}`), state.Metadata{})
		require.NoError(t, err)
		assert.Equal(t, "trigger_payloads", task.Config.TaskType)
		assert.Equal(t, "a_uuid", task.Config.UUID)
		assert.Equal(t, map[string]string{"payloads": "inventory-agent,agent-health"}, task.Config.TaskArgs)
	})

	t.Run("no args", func(t *testing.T) {
		task, err := ParseConfigAgentTask([]byte(`{"task_type":"trigger_payloads","uuid":"a_uuid"}`), state.Metadata{})
		require.NoError(t, err)
		assert.Nil(t, task.Config.TaskArgs)
	})

	t.Run("non-string arg", func(t *testing.T) {
		_, err := ParseConfigAgentTask([]byte(`{"task_type":"trigger_payloads","uuid":"a_uuid","args":{"payloads":["agent-health"]}}`), state.Metadata{})
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
