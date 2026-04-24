// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func ptrInt64(v int64) *int64       { return &v }
func ptrFloat64(v float64) *float64 { return &v }
func ptrBool(v bool) *bool          { return &v }
