// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

func makeContext(str string, label Label) *messageContext {
	tokenizer := NewTokenizer(0)
	ts, _ := tokenizer.tokenize([]byte(str))

	return &messageContext{
		rawMessage: []byte(str),
		tokens:     ts,
		label:      label,
	}
}

func TestPatternTable(t *testing.T) {

	pt := NewPatternTable(5, 1, status.NewInfoRegistry())

	pt.insert(makeContext("abc 123 !", aggregate))
	pt.insert(makeContext("abc 123 @", aggregate))
	pt.insert(makeContext("abc 123 $", aggregate))
	pt.insert(makeContext("abc 123 %", aggregate))
	pt.insert(makeContext("abc 123 ^", aggregate))

	assert.Equal(t, 5, len(pt.table))

	// Add more of the same pattern - should remain at the top and get it's count updated
	pt.insert(makeContext("abc 123 !", aggregate))
	pt.insert(makeContext("abc 123 !", aggregate))

	assert.Equal(t, 5, len(pt.table))
	assert.Equal(t, int64(3), pt.table[0].count)

	// At this point `abc 123 @` was the last updated, so it will be evicted first
	pt.insert(makeContext("abc 123 *", aggregate))

	assert.Equal(t, 5, len(pt.table), "Table should not grow past limit")
	dump := pt.DumpTable()

	assert.Equal(t, 5, len(dump))
	assert.Equal(t, "CCC DDD !", dump[0].TokenString)
	assert.Equal(t, "CCC DDD $", dump[1].TokenString)
	assert.Equal(t, "CCC DDD %", dump[2].TokenString)
	assert.Equal(t, "CCC DDD ^", dump[3].TokenString)
	assert.Equal(t, "CCC DDD *", dump[4].TokenString)

	// Should sift up to position #2
	pt.insert(makeContext("abc 123 *", aggregate))

	dump = pt.DumpTable()

	assert.Equal(t, 5, len(dump))
	assert.Equal(t, "CCC DDD !", dump[0].TokenString)
	assert.Equal(t, "CCC DDD *", dump[1].TokenString)
	assert.Equal(t, "CCC DDD $", dump[2].TokenString)
	assert.Equal(t, "CCC DDD %", dump[3].TokenString)
	assert.Equal(t, "CCC DDD ^", dump[4].TokenString)

	assert.Equal(t, int64(3), dump[0].Count)
	assert.Equal(t, int64(2), dump[1].Count)
	assert.Equal(t, int64(1), dump[2].Count)
	assert.Equal(t, int64(1), dump[3].Count)
	assert.Equal(t, int64(1), dump[4].Count)

	// Lets pretend the whole log format totally changes for some reason, and evict the whole table.
	pt.insert(makeContext("! acb 123", startGroup))
	pt.insert(makeContext("@ acb 123", aggregate))
	pt.insert(makeContext("# acb 123", noAggregate))
	pt.insert(makeContext("$ acb 123", aggregate))
	pt.insert(makeContext("% acb 123", startGroup))

	dump = pt.DumpTable()

	assert.Equal(t, 5, len(dump))
	assert.Equal(t, "! CCC DDD", dump[0].TokenString)
	assert.Equal(t, "@ CCC DDD", dump[1].TokenString)
	assert.Equal(t, "# CCC DDD", dump[2].TokenString)
	assert.Equal(t, "$ CCC DDD", dump[3].TokenString)
	assert.Equal(t, "% CCC DDD", dump[4].TokenString)
	assert.Equal(t, int64(1), dump[0].Count)
	assert.Equal(t, int64(1), dump[1].Count)
	assert.Equal(t, int64(1), dump[2].Count)
	assert.Equal(t, int64(1), dump[3].Count)
	assert.Equal(t, int64(1), dump[4].Count)
	assert.Equal(t, "start_group", dump[0].LabelString)
	assert.Equal(t, "aggregate", dump[1].LabelString)
	assert.Equal(t, "no_aggregate", dump[2].LabelString)
	assert.Equal(t, "aggregate", dump[3].LabelString)
	assert.Equal(t, "start_group", dump[4].LabelString)
}
