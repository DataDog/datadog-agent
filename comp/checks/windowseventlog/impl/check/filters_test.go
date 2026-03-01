// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryFromFilter(t *testing.T) {

	tcs := []struct {
		name   string
		filter filterDefinition
		query  string
	}{
		{"no filters", nil, "*"},
		{"no filters", &filtersConfig{}, "*"},
		{"source single", &filtersConfig{SourceList: []string{"source1"}}, "*[System[Provider[@Name='source1']]]"},
		{"source multiple", &filtersConfig{SourceList: []string{"source1", "source2"}}, "*[System[Provider[(@Name='source1' or @Name='source2')]]]"},
		{"type critical", &filtersConfig{TypeList: []string{"critical"}}, "*[System[Level=1]]"},
		{"type error", &filtersConfig{TypeList: []string{"error"}}, "*[System[Level=2]]"},
		{"type warning", &filtersConfig{TypeList: []string{"warning"}}, "*[System[Level=3]]"},
		{"type information", &filtersConfig{TypeList: []string{"information"}}, "*[System[(Level=0 or Level=4)]]"},
		{"type Success Audit", &filtersConfig{TypeList: []string{"success audit"}}, "*[System[band(Keywords,9007199254740992)]]"},
		{"type Failure Audit", &filtersConfig{TypeList: []string{"failure audit"}}, "*[System[band(Keywords,4503599627370496)]]"},
		{"type multiple", &filtersConfig{TypeList: []string{"critical", "error"}}, "*[System[(Level=1 or Level=2)]]"},
		{"type multiple", &filtersConfig{TypeList: []string{"critical", "information"}}, "*[System[(Level=1 or (Level=0 or Level=4))]]"},
		{"type multiple", &filtersConfig{TypeList: []string{"critical", "information", "Error", "Warning", "Success Audit", "Failure Audit"}},
			"*[System[(Level=1 or (Level=0 or Level=4) or Level=2 or Level=3 or band(Keywords,9007199254740992) or band(Keywords,4503599627370496))]]"},
		{"id single", &filtersConfig{IDList: []int{1000}}, "*[System[EventID=1000]]"},
		{"id multiple", &filtersConfig{IDList: []int{1000, 1001}}, "*[System[(EventID=1000 or EventID=1001)]]"},
		{"complex", &filtersConfig{
			SourceList: []string{"source1", "source2"},
			TypeList:   []string{"critical", "error"},
			IDList:     []int{1000, 1001}},
			"*[System[(Provider[(@Name='source1' or @Name='source2')] and (Level=1 or Level=2) and (EventID=1000 or EventID=1001))]]"},
	}
	for _, tc := range tcs {
		query, err := queryFromFilter(tc.filter)
		assert.NoError(t, err)
		assert.Equal(t, tc.query, query, tc.name)
	}
}
