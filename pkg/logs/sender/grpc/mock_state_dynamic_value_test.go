// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	rtokenizer "github.com/DataDog/datadog-agent/pkg/logs/patterns/tokenizer/rust"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

func TestEncodeDynamicValue_UsesLosslessTypedEncodingsForStringValues(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())

	tests := []struct {
		name               string
		value              string
		wantInt            *int64
		wantFloat          *float64
		wantBool           *bool
		wantRenderAsString bool
		wantDictValue      string
	}{
		{
			name:               "canonical integer",
			value:              "446",
			wantInt:            ptrInt64(446),
			wantRenderAsString: true,
		},
		{
			name:               "canonical float",
			value:              "99.99",
			wantFloat:          ptrFloat64(99.99),
			wantRenderAsString: true,
		},
		{
			name:               "canonical bool",
			value:              "true",
			wantBool:           ptrBool(true),
			wantRenderAsString: true,
		},
		{
			name:          "leading zeros stay string-backed",
			value:         "00123",
			wantDictValue: "00123",
		},
		{
			name:          "scientific notation stays string-backed",
			value:         "1e3",
			wantDictValue: "1e3",
		},
		{
			name:          "float-like value stays string-backed",
			value:         "1.0",
			wantDictValue: "1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, dictID, isNew := mt.encodeDynamicValue(tt.value)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantRenderAsString, got.RenderAsString)

			if tt.wantInt != nil {
				intValue, ok := got.Value.(*statefulpb.DynamicValue_IntValue)
				require.True(t, ok)
				assert.EqualValues(t, *tt.wantInt, intValue.IntValue)
				assert.Zero(t, dictID)
				assert.False(t, isNew)
				return
			}
			if tt.wantFloat != nil {
				floatValue, ok := got.Value.(*statefulpb.DynamicValue_FloatValue)
				require.True(t, ok)
				assert.Equal(t, *tt.wantFloat, floatValue.FloatValue)
				assert.Zero(t, dictID)
				assert.False(t, isNew)
				return
			}
			if tt.wantBool != nil {
				boolValue, ok := got.Value.(*statefulpb.DynamicValue_BoolValue)
				require.True(t, ok)
				assert.Equal(t, *tt.wantBool, boolValue.BoolValue)
				assert.Zero(t, dictID)
				assert.False(t, isNew)
				return
			}

			dictIndex, ok := got.Value.(*statefulpb.DynamicValue_DictIndex)
			require.True(t, ok)
			assert.NotZero(t, dictID)
			assert.Equal(t, dictID, dictIndex.DictIndex)
			assert.True(t, isNew)
			lookedUpID, exists := mt.tagManager.GetStringID(tt.wantDictValue)
			require.True(t, exists)
			assert.Equal(t, dictID, lookedUpID)
		})
	}
}

func TestFillDynamicValue_PreservesNonCanonicalNumericStrings(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())

	var dv statefulpb.DynamicValue
	var intOneof statefulpb.DynamicValue_IntValue
	var floatOneof statefulpb.DynamicValue_FloatValue
	var boolOneof statefulpb.DynamicValue_BoolValue
	var dictOneof statefulpb.DynamicValue_DictIndex
	var rawJSONOneof statefulpb.DynamicValue_RawJsonValue
	var strOneof statefulpb.DynamicValue_StringValue

	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, "00123")
	stringValue, ok := dv.Value.(*statefulpb.DynamicValue_StringValue)
	require.True(t, ok)
	assert.Equal(t, "00123", stringValue.StringValue)
	assert.False(t, dv.RenderAsString)

	dictID, _ := mt.tagManager.AddString("00123")
	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, "00123")
	dictValue, ok := dv.Value.(*statefulpb.DynamicValue_DictIndex)
	require.True(t, ok)
	assert.Equal(t, dictID, dictValue.DictIndex)
	assert.False(t, dv.RenderAsString)

	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, "446")
	intValue, ok := dv.Value.(*statefulpb.DynamicValue_IntValue)
	require.True(t, ok)
	assert.EqualValues(t, 446, intValue.IntValue)
	assert.True(t, dv.RenderAsString)
}

func TestFillDynamicValue_AddsRepeatedStringsToDictionary(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())

	var dv statefulpb.DynamicValue
	var intOneof statefulpb.DynamicValue_IntValue
	var floatOneof statefulpb.DynamicValue_FloatValue
	var boolOneof statefulpb.DynamicValue_BoolValue
	var dictOneof statefulpb.DynamicValue_DictIndex
	var rawJSONOneof statefulpb.DynamicValue_RawJsonValue
	var strOneof statefulpb.DynamicValue_StringValue

	dictID, dictValue, isNew := mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, "INFO")
	stringValue, ok := dv.Value.(*statefulpb.DynamicValue_StringValue)
	require.True(t, ok)
	assert.Equal(t, "INFO", stringValue.StringValue)
	assert.Zero(t, dictID)
	assert.Empty(t, dictValue)
	assert.False(t, isNew)
	assert.Equal(t, 0, mt.tagManager.Count())

	dictID, dictValue, isNew = mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, "INFO")
	dictIndex, ok := dv.Value.(*statefulpb.DynamicValue_DictIndex)
	require.True(t, ok)
	assert.Equal(t, dictID, dictIndex.DictIndex)
	assert.Equal(t, "INFO", dictValue)
	assert.True(t, isNew)
	assert.Equal(t, 1, mt.tagManager.Count())

	dictIDAgain, dictValue, isNew := mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, "INFO")
	dictIndex, ok = dv.Value.(*statefulpb.DynamicValue_DictIndex)
	require.True(t, ok)
	assert.Equal(t, dictID, dictIDAgain)
	assert.Equal(t, dictID, dictIndex.DictIndex)
	assert.Equal(t, "INFO", dictValue)
	assert.False(t, isNew)
}

func TestFillWildcardDynamicValue_AddsRepeatedStringsToDictionary(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())

	var dv statefulpb.DynamicValue
	var intOneof statefulpb.DynamicValue_IntValue
	var dictOneof statefulpb.DynamicValue_DictIndex
	var strOneof statefulpb.DynamicValue_StringValue

	dictID, dictValue, isNew := mt.fillWildcardDynamicValue(&dv, &intOneof, &dictOneof, &strOneof, "INFO")
	stringValue, ok := dv.Value.(*statefulpb.DynamicValue_StringValue)
	require.True(t, ok)
	assert.Equal(t, "INFO", stringValue.StringValue)
	assert.Zero(t, dictID)
	assert.Empty(t, dictValue)
	assert.False(t, isNew)

	dictID, dictValue, isNew = mt.fillWildcardDynamicValue(&dv, &intOneof, &dictOneof, &strOneof, "INFO")
	dictIndex, ok := dv.Value.(*statefulpb.DynamicValue_DictIndex)
	require.True(t, ok)
	assert.Equal(t, dictID, dictIndex.DictIndex)
	assert.Equal(t, "INFO", dictValue)
	assert.True(t, isNew)
}

func TestFillWildcardDynamicValue_DoesNotDictionaryEncodeUUIDsOrTimestamps(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())

	var dv statefulpb.DynamicValue
	var intOneof statefulpb.DynamicValue_IntValue
	var dictOneof statefulpb.DynamicValue_DictIndex
	var strOneof statefulpb.DynamicValue_StringValue

	values := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"2026-04-28T12:34:56Z",
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			for i := 0; i < 3; i++ {
				dictID, dictValue, isNew := mt.fillWildcardDynamicValue(&dv, &intOneof, &dictOneof, &strOneof, value)
				stringValue, ok := dv.Value.(*statefulpb.DynamicValue_StringValue)
				require.True(t, ok)
				assert.Equal(t, value, stringValue.StringValue)
				assert.Zero(t, dictID)
				assert.Empty(t, dictValue)
				assert.False(t, isNew)
			}
		})
	}
	assert.Equal(t, 0, mt.tagManager.Count())
}

func TestFillDynamicValue_PreservesTypedJSONValues(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())

	var dv statefulpb.DynamicValue
	var intOneof statefulpb.DynamicValue_IntValue
	var floatOneof statefulpb.DynamicValue_FloatValue
	var boolOneof statefulpb.DynamicValue_BoolValue
	var dictOneof statefulpb.DynamicValue_DictIndex
	var rawJSONOneof statefulpb.DynamicValue_RawJsonValue
	var strOneof statefulpb.DynamicValue_StringValue

	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, float64(42))
	intValue, ok := dv.Value.(*statefulpb.DynamicValue_IntValue)
	require.True(t, ok)
	assert.EqualValues(t, 42, intValue.IntValue)
	assert.False(t, dv.RenderAsString)

	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, float64(42.5))
	floatValue, ok := dv.Value.(*statefulpb.DynamicValue_FloatValue)
	require.True(t, ok)
	assert.Equal(t, 42.5, floatValue.FloatValue)
	assert.False(t, dv.RenderAsString)

	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, true)
	boolValue, ok := dv.Value.(*statefulpb.DynamicValue_BoolValue)
	require.True(t, ok)
	assert.True(t, boolValue.BoolValue)
	assert.False(t, dv.RenderAsString)

	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, nil)
	assert.Nil(t, dv.Value)
	assert.False(t, dv.RenderAsString)
}

func TestFillDynamicValue_JSONNumberPreservesLargeIntegerPrecision(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())

	var dv statefulpb.DynamicValue
	var intOneof statefulpb.DynamicValue_IntValue
	var floatOneof statefulpb.DynamicValue_FloatValue
	var boolOneof statefulpb.DynamicValue_BoolValue
	var dictOneof statefulpb.DynamicValue_DictIndex
	var rawJSONOneof statefulpb.DynamicValue_RawJsonValue
	var strOneof statefulpb.DynamicValue_StringValue

	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, json.Number("2323969980879066318"))
	intValue, ok := dv.Value.(*statefulpb.DynamicValue_IntValue)
	require.True(t, ok)
	assert.EqualValues(t, 2323969980879066318, intValue.IntValue)
	assert.False(t, dv.RenderAsString)
}

func TestFillDynamicValue_JSONNumberFallsBackToRawJSONWhenLossy(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())

	var dv statefulpb.DynamicValue
	var intOneof statefulpb.DynamicValue_IntValue
	var floatOneof statefulpb.DynamicValue_FloatValue
	var boolOneof statefulpb.DynamicValue_BoolValue
	var dictOneof statefulpb.DynamicValue_DictIndex
	var rawJSONOneof statefulpb.DynamicValue_RawJsonValue
	var strOneof statefulpb.DynamicValue_StringValue

	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, json.Number("16363125235853554000"))
	rawJSONValue, ok := dv.Value.(*statefulpb.DynamicValue_RawJsonValue)
	require.True(t, ok)
	assert.Equal(t, []byte("16363125235853554000"), rawJSONValue.RawJsonValue)
	assert.False(t, dv.RenderAsString)
}

func TestFillDynamicValue_JSONNumberUsesFloatWhenLossless(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())

	var dv statefulpb.DynamicValue
	var intOneof statefulpb.DynamicValue_IntValue
	var floatOneof statefulpb.DynamicValue_FloatValue
	var boolOneof statefulpb.DynamicValue_BoolValue
	var dictOneof statefulpb.DynamicValue_DictIndex
	var rawJSONOneof statefulpb.DynamicValue_RawJsonValue
	var strOneof statefulpb.DynamicValue_StringValue

	mt.fillDynamicValue(&dv, &intOneof, &floatOneof, &boolOneof, &dictOneof, &rawJSONOneof, &strOneof, json.Number("159.6"))
	floatValue, ok := dv.Value.(*statefulpb.DynamicValue_FloatValue)
	require.True(t, ok)
	assert.Equal(t, 159.6, floatValue.FloatValue)
	assert.False(t, dv.RenderAsString)
}

func TestJsonSchemaReuseRefreshesReferencedDictEntries(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())
	outputChan := make(chan *message.StatefulMessage, 10)
	msg := message.NewMessage([]byte("request done"), nil, "", 0)

	_, schemaID := mt.sendJsonSchemaDefineIfNeeded(outputChan, msg, "msg", []string{"level", "object.kind"})
	require.NotZero(t, schemaID)
	firstSchema := mt.jsonSchemaByID[schemaID]
	require.NotNil(t, firstSchema)

	time.Sleep(20 * time.Millisecond)
	_, reusedSchemaID := mt.sendJsonSchemaDefineIfNeeded(outputChan, msg, "msg", []string{"level", "object.kind"})
	require.Equal(t, schemaID, reusedSchemaID)

	evictedIDs := mt.tagManager.EvictStaleEntries(10 * time.Millisecond)
	assert.NotContains(t, evictedIDs, firstSchema.messageKeyID)
	for _, keyID := range firstSchema.keyIDs {
		assert.NotContains(t, evictedIDs, keyID)
	}
}

func TestJsonSchemaDeletedWhenReferencedDictEntryEvicted(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())
	outputChan := make(chan *message.StatefulMessage, 10)
	msg := message.NewMessage([]byte("request done"), nil, "", 0)

	_, schemaID := mt.sendJsonSchemaDefineIfNeeded(outputChan, msg, "msg", []string{"level", "object.kind"})
	require.NotZero(t, schemaID)
	firstSchema := mt.jsonSchemaByID[schemaID]
	require.NotNil(t, firstSchema)

	evictedIDs := mt.tagManager.EvictStaleEntries(0)
	require.Contains(t, evictedIDs, firstSchema.keyIDs[0])
	mt.sendDictEntryDeletes(outputChan, msg, evictedIDs)

	require.NotContains(t, mt.jsonSchemaByID, schemaID)
	require.NotContains(t, mt.jsonSchemaToID, firstSchema.schemaKey)

	var sawSchemaDelete bool
	var sawDictDelete bool
	for len(outputChan) > 0 {
		statefulMsg := <-outputChan
		if schemaDelete := statefulMsg.Datum.GetJsonSchemaDelete(); schemaDelete != nil && schemaDelete.SchemaId == schemaID {
			sawSchemaDelete = true
		}
		if dictDelete := statefulMsg.Datum.GetDictEntryDelete(); dictDelete != nil && dictDelete.Id == firstSchema.keyIDs[0] {
			sawDictDelete = true
			assert.True(t, sawSchemaDelete, "json schema must be deleted before the referenced dict entry")
			break
		}
	}
	assert.True(t, sawSchemaDelete, "evicting a referenced dict entry must delete dependent json schema")
	assert.True(t, sawDictDelete, "referenced dict entry should be deleted after dependent json schema")
}

func TestJsonSchemaRedefinedWhenReferencedDictIDsChange(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())
	outputChan := make(chan *message.StatefulMessage, 20)
	msg := message.NewMessage([]byte("request done"), nil, "", 0)

	_, schemaID := mt.sendJsonSchemaDefineIfNeeded(outputChan, msg, "msg", []string{"level", "object.kind"})
	require.NotZero(t, schemaID)
	firstSchema := mt.jsonSchemaByID[schemaID]
	require.NotNil(t, firstSchema)

	evictedIDs := mt.tagManager.EvictStaleEntries(0)
	require.Contains(t, evictedIDs, firstSchema.keyIDs[0])
	// Simulate a stale schema cache where the dictionary entries were evicted
	// without the corresponding json schema cache entry being invalidated.

	_, redefinedSchemaID := mt.sendJsonSchemaDefineIfNeeded(outputChan, msg, "msg", []string{"level", "object.kind"})
	require.NotEqual(t, schemaID, redefinedSchemaID)
	redefinedSchema := mt.jsonSchemaByID[redefinedSchemaID]
	require.NotNil(t, redefinedSchema)
	assert.NotEqual(t, firstSchema.keyIDs, redefinedSchema.keyIDs)

	var sawOldSchemaDelete bool
	var sawNewSchemaDefine bool
	for len(outputChan) > 0 {
		statefulMsg := <-outputChan
		if schemaDelete := statefulMsg.Datum.GetJsonSchemaDelete(); schemaDelete != nil && schemaDelete.SchemaId == schemaID {
			sawOldSchemaDelete = true
		}
		if schemaDefine := statefulMsg.Datum.GetJsonSchemaDefine(); schemaDefine != nil && schemaDefine.SchemaId == redefinedSchemaID {
			sawNewSchemaDefine = true
		}
	}
	assert.True(t, sawOldSchemaDelete)
	assert.True(t, sawNewSchemaDefine)
}

func TestJsonSchemaDefineKeepsEmptyKeysAlignedWithCompactValues(t *testing.T) {
	mt := NewMessageTranslator("test-pipeline", rtokenizer.NewRustTokenizer())
	outputChan := make(chan *message.StatefulMessage, 10)
	msg := message.NewMessage([]byte("request done"), nil, "", 0)

	_, schemaID := mt.sendJsonSchemaDefineIfNeeded(outputChan, msg, "message", []string{"", "level"})
	require.NotZero(t, schemaID)
	schema := mt.jsonSchemaByID[schemaID]
	require.NotNil(t, schema)
	require.Len(t, schema.keyIDs, 2)

	compact, _ := mt.compactJSONContextValues([]interface{}{"empty-key-value", "INFO"})
	assert.Len(t, compact.kinds, len(schema.keyIDs))
}

func ptrInt64(v int64) *int64       { return &v }
func ptrFloat64(v float64) *float64 { return &v }
func ptrBool(v bool) *bool          { return &v }
