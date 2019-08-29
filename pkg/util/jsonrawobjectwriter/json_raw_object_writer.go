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
	return writer
}

// EmptyPolicy defines the behavior when adding an empty string
type EmptyPolicy int

const (
	// OmitEmpty does not write the field if the string is empty
	OmitEmpty EmptyPolicy = iota

	// AllowEmpty writes the string even if the string is empty
	AllowEmpty
)

// StartObject starts a new JSON object (add '{')
func (writer *JSONRawObjectWriter) StartObject() {
	writer.stream.WriteObjectStart()
	writer.addScope()
}

// FinishObject finishes a JSON object (add '}')
func (writer *JSONRawObjectWriter) FinishObject() {
	writer.stream.WriteObjectEnd()
}

// AddStringField adds a new field of type string
func (writer *JSONRawObjectWriter) AddStringField(fieldName string, value string, policy EmptyPolicy) {
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

// Flush the stream
func (writer *JSONRawObjectWriter) Flush() error {
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
