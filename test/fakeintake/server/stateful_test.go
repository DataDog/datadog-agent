// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

// appendDictEntryDefine appends a LogDatum{dict_entry_define=1} wrapping a
// DictEntryDefine{id=1, value=2}.
func appendDictEntryDefine(buf []byte, id uint64, value string) []byte {
	entry := protowire.AppendTag(nil, 1, protowire.VarintType)
	entry = protowire.AppendVarint(entry, id)
	entry = protowire.AppendTag(entry, 2, protowire.BytesType)
	entry = protowire.AppendBytes(entry, []byte(value))

	buf = protowire.AppendTag(buf, 1, protowire.BytesType)
	return protowire.AppendBytes(buf, entry)
}

// appendLogDatum appends a LogDatum{log=4} wrapping a Log with the given
// service id (field 3), tags id (field 4), and raw_log (field 7). A zero id
// omits that field, matching how the client only sends it when it changes.
func appendLogDatum(buf []byte, serviceID, tagsID uint64, rawLog string) []byte {
	var logMsg []byte
	if serviceID != 0 {
		logMsg = protowire.AppendTag(logMsg, 3, protowire.VarintType)
		logMsg = protowire.AppendVarint(logMsg, serviceID)
	}
	if tagsID != 0 {
		logMsg = protowire.AppendTag(logMsg, 4, protowire.VarintType)
		logMsg = protowire.AppendVarint(logMsg, tagsID)
	}
	logMsg = protowire.AppendTag(logMsg, 7, protowire.BytesType)
	logMsg = protowire.AppendBytes(logMsg, []byte(rawLog))

	buf = protowire.AppendTag(buf, 4, protowire.BytesType)
	return protowire.AppendBytes(buf, logMsg)
}

func TestStatefulDecodeLogsResolvesDictionaryAndDeltaService(t *testing.T) {
	state := newStatefulDecodeState()

	// Batch 1: define the service dict entry and use it in the same batch.
	var batch1 []byte
	batch1 = appendDictEntryDefine(batch1, 2, "hello")
	batch1 = appendLogDatum(batch1, 2, 0, "first log")

	logs := state.decodeLogs(batch1)
	require.Len(t, logs, 1)
	assert.Equal(t, "first log", logs[0].Message)
	assert.Equal(t, "hello", logs[0].Service)

	// Batch 2: service id 0 means "reuse last" (the same dict-resolved
	// service), without redefining or resending the dict id.
	var batch2 []byte
	batch2 = appendLogDatum(batch2, 0, 0, "second log")

	logs = state.decodeLogs(batch2)
	require.Len(t, logs, 1)
	assert.Equal(t, "second log", logs[0].Message)
	assert.Equal(t, "hello", logs[0].Service, "service id 0 should carry forward the last resolved service across batches")
}

func TestStatefulDecodeLogsServiceIDOneMeansAbsent(t *testing.T) {
	state := newStatefulDecodeState()

	var batch []byte
	batch = appendLogDatum(batch, 1, 0, "no service")

	logs := state.decodeLogs(batch)
	require.Len(t, logs, 1)
	assert.Empty(t, logs[0].Service)
}

func TestStatefulDecodeLogsTagsResolveThroughDictionary(t *testing.T) {
	state := newStatefulDecodeState()

	var batch []byte
	batch = appendDictEntryDefine(batch, 2, "env:prod,team:logs")
	batch = appendLogDatum(batch, 0, 2, "tagged log")

	logs := state.decodeLogs(batch)
	require.Len(t, logs, 1)
	assert.ElementsMatch(t, []string{"env:prod", "team:logs"}, []string(logs[0].Tags))
}

func TestStatefulDecodeLogsSkipsBlankRawLog(t *testing.T) {
	state := newStatefulDecodeState()

	var batch []byte
	batch = appendLogDatum(batch, 0, 0, "  ")

	logs := state.decodeLogs(batch)
	assert.Empty(t, logs)
}
