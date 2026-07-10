// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package powershell

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToFloat(t *testing.T) {
	cases := []struct {
		in   interface{}
		want float64
		ok   bool
	}{
		{float64(4), 4, true}, // enums decode to their integer value
		{float64(1.5), 1.5, true},
		{true, 1, true},
		{false, 0, true},
		{"42", 42, true},
		{"  7 ", 7, true},
		{"not-a-number", 0, false},
		{nil, 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := toFloat(c.in)
		assert.Equal(t, c.ok, ok, "input %v", c.in)
		if c.ok {
			assert.Equal(t, c.want, got, "input %v", c.in)
		}
	}
}

func TestTagValue(t *testing.T) {
	assert.Equal(t, "", tagValue(nil))
	assert.Equal(t, "running", tagValue("running"))
	assert.Equal(t, "true", tagValue(true))
	assert.Equal(t, "5", tagValue(float64(5)))
	assert.Equal(t, "1.5", tagValue(float64(1.5)))
}

func TestBuildTags(t *testing.T) {
	inst := &instanceConfig{
		TagBy: []tagByEntry{
			{Property: "Name", Alias: "node"},
			{Property: "State", Alias: "state"},
			{Property: "Missing", Alias: "missing"}, // absent -> skipped
		},
		Tags: []string{"role:db"},
		TagQueries: []tagQueryEntry{
			{LinkSourceProperty: "Id", TargetCmdlet: "Get-ClusterGroup", LinkTargetProperty: "OwnerNode", TargetProperty: "Name", Alias: "owner_group"},
		},
	}
	row := map[string]interface{}{
		"Name":  "node1",
		"State": "Up",
		"Id":    float64(7),
	}
	joins := []map[string]string{
		{"7": "web-group"},
	}

	tags := buildTags(inst, row, joins)
	assert.ElementsMatch(t, []string{
		"node:node1",
		"state:Up",
		"role:db",
		"owner_group:web-group",
	}, tags)
}

func TestBuildTagsNoJoinMatch(t *testing.T) {
	inst := &instanceConfig{
		TagQueries: []tagQueryEntry{
			{LinkSourceProperty: "Id", TargetCmdlet: "Get-X", LinkTargetProperty: "Y", TargetProperty: "Z", Alias: "z"},
		},
	}
	row := map[string]interface{}{"Id": float64(99)}
	joins := []map[string]string{{"7": "web-group"}}
	tags := buildTags(inst, row, joins)
	assert.Empty(t, tags)
}
