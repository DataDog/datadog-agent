// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

package jsonrawobjectwriter

import jsoniter "github.com/json-iterator/go"

// JSONRawObjectWriter contains helper functions to write JSON using the raw API.
type JSONRawObjectWriter struct {
	stream                  *jsoniter.Stream
	requireSeparatorByScope []bool
}

// NewJSONRawObjectWriter creates a new instance of JSONRawObjectWriter
func NewJSONRawObjectWriter(stream *jsoniter.Stream) *JSONRawObjectWriter {
	writer := &JSONRawObjectWriter{stream: stream}
	writer.stream.WriteObjectStart()
	writer.addScope()
	return writer
}

// JSONRawObjectWriterEmptyPolicy defines the behavior when adding an empty string
type JSONRawObjectWriterEmptyPolicy int

const (
	// OmitEmpty does not write the field if the string is empty
	OmitEmpty JSONRawObjectWriterEmptyPolicy = iota

	// AllowEmpty writes the string even if the string is empty
	AllowEmpty
)

// AddStringField adds a new field of type string
func (writer *JSONRawObjectWriter) AddStringField(fieldName string, value string, policy JSONRawObjectWriterEmptyPolicy) {
	if value != "" || policy == AllowEmpty {
		writer.writeSeparatorIfNeeded()
		writer.stream.WriteObjectField(fieldName)
		writer.stream.WriteString(value)
	}
}

// AddInt64Field adds a new field of type int64
func (writer *JSONRawObjectWriter) AddInt64Field(fieldName string, value int64) {
	writer.writeSeparatorIfNeeded()
	writer.stream.WriteObjectField(fieldName)
	writer.stream.WriteInt64(value)
}

// StartArrayField starts a new field of type array
func (writer *JSONRawObjectWriter) StartArrayField(fieldName string) {
	writer.writeSeparatorIfNeeded()
	writer.stream.WriteObjectField(fieldName)
	writer.stream.WriteArrayStart()
	writer.addScope()
}

// FinishArrayField finishes an array field
func (writer *JSONRawObjectWriter) FinishArrayField() {
	writer.stream.WriteArrayEnd()
	writer.removeScope()
}

// AddStringValue adds a string (for example inside an array)
func (writer *JSONRawObjectWriter) AddStringValue(value string) {
	writer.writeSeparatorIfNeeded()
	writer.stream.WriteString(value)
}

// Close closes the JSON object and flush the stream
func (writer *JSONRawObjectWriter) Close() error {
	writer.stream.WriteObjectEnd()
	return writer.stream.Flush()
}

func (writer *JSONRawObjectWriter) toString() string {
	return string(writer.stream.Buffer())
}

func (writer *JSONRawObjectWriter) writeSeparatorIfNeeded() {
	if len(writer.requireSeparatorByScope) > 0 {
		index := len(writer.requireSeparatorByScope) - 1
		if writer.requireSeparatorByScope[index] {
			writer.stream.WriteMore()
		} else {
			writer.requireSeparatorByScope[index] = true
		}
	}
}

func (writer *JSONRawObjectWriter) addScope() {
	writer.requireSeparatorByScope = append(writer.requireSeparatorByScope, false)
}

func (writer *JSONRawObjectWriter) removeScope() {
	len := len(writer.requireSeparatorByScope)
	if len > 0 {
		writer.requireSeparatorByScope = writer.requireSeparatorByScope[:len-1]
	}
}
