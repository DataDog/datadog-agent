// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

type rawEventMarshaler []byte

func (m rawEventMarshaler) ToJSON() ([]byte, error) {
	return m, nil
}

func TestCoreDumpToJSONIncludesRuleID(t *testing.T) {
	def := &rules.CoreDumpDefinition{NoCompression: true}
	dump := NewCoreDump(def, nil, rawEventMarshaler(`{"foo":"bar"}`), "my_rule")

	data, err := dump.ToJSON()
	require.NoError(t, err)

	var content struct {
		RuleID string          `json:"rule_id"`
		Event  json.RawMessage `json:"event"`
	}
	require.NoError(t, json.Unmarshal(data, &content))
	assert.Equal(t, "my_rule", content.RuleID)
	assert.JSONEq(t, `{"foo":"bar"}`, string(content.Event))
}
