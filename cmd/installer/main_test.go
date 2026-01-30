// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package main

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/commands"
)

func TestFormatError_HumanReadable(t *testing.T) {
	cmd := &cobra.Command{
		Annotations: map[string]string{commands.AnnotationHumanReadableErrors: "true"},
	}
	err := errors.New("something went wrong")

	result := formatError(cmd, err)

	assert.Equal(t, "something went wrong", result)
	assert.NotContains(t, result, `"code"`)
}

func TestFormatError_JSON(t *testing.T) {
	cmd := &cobra.Command{}
	err := errors.New("something went wrong")

	result := formatError(cmd, err)

	var parsed map[string]interface{}
	unmarshalErr := json.Unmarshal([]byte(result), &parsed)
	assert.NoError(t, unmarshalErr, "result should be valid JSON")
	assert.Contains(t, parsed, "error")
	assert.Contains(t, parsed, "code")
	assert.Equal(t, "something went wrong", parsed["error"])
}

func TestFormatError_NilCommand(t *testing.T) {
	err := errors.New("something went wrong")

	result := formatError(nil, err)

	// Should fall back to JSON when command is nil
	var parsed map[string]interface{}
	unmarshalErr := json.Unmarshal([]byte(result), &parsed)
	assert.NoError(t, unmarshalErr, "result should be valid JSON")
	assert.Contains(t, parsed, "error")
	assert.Equal(t, "something went wrong", parsed["error"])
}
