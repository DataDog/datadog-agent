// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

package jsonrawobjectwriter

import (
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
)

func newJSONRawObjectWriterTest() *JSONRawObjectWriter {
	jsonStream := jsoniter.NewStream(jsoniter.ConfigDefault, nil, 0)

	return NewJSONRawObjectWriter(jsonStream)
}

func TestJSONRawObjectWriterSimpleField(t *testing.T) {
	writer := newJSONRawObjectWriterTest()

	writer.StartObject()
	writer.AddStringField("f1", "1", AllowEmpty)
	writer.AddStringField("f2", "", OmitEmpty)
	writer.AddInt64Field("f3", 3)
	writer.FinishObject()
	writer.Flush()

	assert.Equal(t, `{"f1":"1","f3":3}`, writer.toString())
}

func TestJSONRawObjectWriterStringArray(t *testing.T) {
	writer := newJSONRawObjectWriterTest()

	writer.StartObject()
	writer.StartArrayField("array")
	writer.AddStringValue("1")
	writer.AddStringValue("2")
	writer.AddStringValue("3")
	writer.FinishArrayField()
	writer.FinishObject()
	writer.Flush()

	assert.Equal(t, `{"array":["1","2","3"]}`, writer.toString())
}
