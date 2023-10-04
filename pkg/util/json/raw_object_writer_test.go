// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package json

import (
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
)

func newRawObjectWriterTest() *RawObjectWriter {
	jsonStream := jsoniter.NewStream(jsoniter.ConfigDefault, nil, 0)

	return NewRawObjectWriter(jsonStream)
}

func TestRawObjectWriterSimpleField(t *testing.T) {
	writer := newRawObjectWriterTest()

	assert.NoError(t, writer.StartObject())
	writer.AddStringField("f1", "1", AllowEmpty)
	writer.AddStringField("f2", "", OmitEmpty)
	writer.AddInt64Field("f3", 3)
	assert.NoError(t, writer.FinishObject())
	writer.Flush()

	assert.Equal(t, `{"f1":"1","f3":3}`, writer.toString())
}

func TestRawObjectWriterStringArray(t *testing.T) {
	writer := newRawObjectWriterTest()

	assert.NoError(t, writer.StartObject())
	assert.NoError(t, writer.StartArrayField("array"))
	writer.AddStringValue("1")
	writer.AddStringValue("2")
	writer.AddStringValue("3")
	assert.NoError(t, writer.FinishArrayField())
	assert.NoError(t, writer.FinishObject())
	writer.Flush()

	assert.Equal(t, `{"array":["1","2","3"]}`, writer.toString())
}

func TestRawObjectWriterInvalidScope(t *testing.T) {
	writer := newRawObjectWriterTest()

	assert.NoError(t, writer.StartObject())
	assert.NoError(t, writer.FinishObject())
	assert.Error(t, writer.FinishObject())

	assert.NoError(t, writer.StartArrayField("array"))
	assert.NoError(t, writer.FinishArrayField())
	assert.Error(t, writer.FinishArrayField())
}

func TestRawObjectWriterInnerObjectArray(t *testing.T) {
	writer := newRawObjectWriterTest()

	assert.NoError(t, writer.StartObject())
	assert.NoError(t, writer.StartObject())
	assert.NoError(t, writer.StartArrayField("array"))

	assert.NoError(t, writer.FinishArrayField())
	assert.NoError(t, writer.FinishObject())
	assert.NoError(t, writer.FinishObject())

	writer.Flush()

	assert.Equal(t, `{{"array":[]}}`, writer.toString())
}
