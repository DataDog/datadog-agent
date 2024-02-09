// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"encoding/json"

	"github.com/mailru/easyjson/jwriter"
)

// StringKeys is a map of strings, that serialize to JSON as an array of strings
type StringKeys struct {
	inner map[string]struct{}
}

// NewStringKeys returns a new `StringKeys` build from the provided keys
func NewStringKeys(from []string) *StringKeys {
	sk := &StringKeys{
		inner: make(map[string]struct{}, len(from)),
	}

	for _, value := range from {
		sk.inner[value] = struct{}{}
	}
	return sk
}

// Insert inserts a new key in the map
func (sk *StringKeys) Insert(value string) {
	sk.inner[value] = struct{}{}
}

// ForEach iterates over each key, and run `f` on them
func (sk *StringKeys) ForEach(f func(string)) {
	for value := range sk.inner {
		f(value)
	}
}

// Keys returns a slice of all the keys contained in this map
func (sk *StringKeys) Keys() []string {
	values := make([]string, 0, len(sk.inner))
	for value := range sk.inner {
		values = append(values, value)
	}
	return values
}

// MarshalJSON marshals the keys into JSON
func (sk *StringKeys) MarshalJSON() ([]byte, error) {
	return json.Marshal(sk.Keys())
}

// MarshalEasyJSON marshals the keys into JSON, using easyJSON
func (sk *StringKeys) MarshalEasyJSON(out *jwriter.Writer) {
	if len(sk.inner) == 0 && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
		out.RawString("null")
		return
	}

	out.RawByte('[')
	isFirst := true
	for value := range sk.inner {
		if !isFirst {
			out.RawByte(',')
		}
		out.String(value)
		isFirst = false
	}
	out.RawByte(']')
}
