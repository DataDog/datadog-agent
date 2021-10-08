// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package json

import (
	"errors"

	jsoniter "github.com/json-iterator/go"
)

// RawObjectWriter contains helper functions to write JSON using the raw API.
type RawObjectWriter struct {
	stream                  *jsoniter.Stream
	requireSeparatorByScope [8]bool
	scopeLevel              int
}

// NewRawObjectWriter creates a new instance of RawObjectWriter
func NewRawObjectWriter(stream *jsoniter.Stream) *RawObjectWriter {
	writer := &RawObjectWriter{stream: stream, scopeLevel: -1}
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
func (writer *RawObjectWriter) StartObject() error {
	writer.writeSeparatorIfNeeded()
	writer.stream.WriteObjectStart()
	return writer.addScope()
}

// FinishObject finishes a JSON object (add '}')
func (writer *RawObjectWriter) FinishObject() error {
	writer.stream.WriteObjectEnd()
	return writer.removeScope()
}

// AddStringField adds a new field of type string
func (writer *RawObjectWriter) AddStringField(fieldName string, value string, policy EmptyPolicy) {
	if value != "" || policy == AllowEmpty {
		writer.writeSeparatorIfNeeded()
		writer.stream.WriteObjectField(fieldName)
		writer.stream.WriteString(value)
	}
}

// AddInt64Field adds a new field of type int64
func (writer *RawObjectWriter) AddInt64Field(fieldName string, value int64) {
	writer.writeSeparatorIfNeeded()
	writer.stream.WriteObjectField(fieldName)
	writer.stream.WriteInt64(value)
}

// StartArrayField starts a new field of type array
func (writer *RawObjectWriter) StartArrayField(fieldName string) error {
	writer.writeSeparatorIfNeeded()
	writer.stream.WriteObjectField(fieldName)
	writer.stream.WriteArrayStart()
	return writer.addScope()
}

// FinishArrayField finishes an array field
func (writer *RawObjectWriter) FinishArrayField() error {
	writer.stream.WriteArrayEnd()
	return writer.removeScope()
}

// AddStringValue adds a string (for example inside an array)
func (writer *RawObjectWriter) AddStringValue(value string) {
	writer.writeSeparatorIfNeeded()
	writer.stream.WriteString(value)
}

// Flush the stream
func (writer *RawObjectWriter) Flush() error {
	return writer.stream.Flush()
}

func (writer *RawObjectWriter) toString() string {
	return string(writer.stream.Buffer())
}

func (writer *RawObjectWriter) writeSeparatorIfNeeded() {
	if writer.scopeLevel >= 0 && writer.scopeLevel < len(writer.requireSeparatorByScope) {
		if writer.requireSeparatorByScope[writer.scopeLevel] {
			writer.stream.WriteMore()
		} else {
			writer.requireSeparatorByScope[writer.scopeLevel] = true
		}
	}
}

func (writer *RawObjectWriter) addScope() error {
	writer.scopeLevel++
	if writer.scopeLevel >= len(writer.requireSeparatorByScope) {
		return errors.New("Too many scopes")
	}
	writer.requireSeparatorByScope[writer.scopeLevel] = false
	return nil
}

func (writer *RawObjectWriter) removeScope() error {
	if writer.scopeLevel < 0 {
		return errors.New("No scope to remove")
	}
	writer.scopeLevel--
	return nil
}
