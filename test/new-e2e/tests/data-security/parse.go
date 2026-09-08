// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package datasecurity contains e2e tests for the packaged Data Security shared-library check.
package datasecurity

import (
	"bytes"
	"encoding/json"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
)

// epEvent is one event-platform payload from `agent check --json`.
type epEvent struct {
	EventType string `json:"EventType"`
	RawEvent  string `json:"RawEvent"`
}

// sdsExtra holds event-platform fields that check.Root does not model.
type sdsExtra struct {
	Aggregator struct {
		SDSResults []epEvent `json:"sds-result"`
	} `json:"aggregator"`
	Runner struct {
		EventPlatformEvents map[string]int `json:"EventPlatformEvents"`
	} `json:"runner"`
}

// parseCheckOutput returns check.Root plus this check's event-platform fields.
// TODO(DATASEC): fold these fields into check.Root and use check.ParseJSONOutput.
func parseCheckOutput(t require.TestingT, out []byte) (check.Root, sdsExtra) {
	startIdx := bytes.IndexAny(out, "[{")
	require.NotEqual(t, -1, startIdx, "no JSON in check output: %s", out)
	trimmed := out[startIdx:]

	var roots []check.Root
	require.NoError(t, json.Unmarshal(trimmed, &roots), "failed to unmarshal check output: %s", out)
	require.NotEmpty(t, roots, "empty check JSON: %s", out)

	var extras []sdsExtra
	require.NoError(t, json.Unmarshal(trimmed, &extras), "failed to unmarshal check output: %s", out)

	return roots[0], extras[0]
}
