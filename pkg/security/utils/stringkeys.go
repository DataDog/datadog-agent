// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"

	"github.com/mailru/easyjson/jwriter"
)

// StringKeys is a map of strings, that serialize to JSON as an array of strings
type StringKeys struct {
	inner map[string]struct{}
}

func NewStringKeys(from []string) *StringKeys {
	sk := &StringKeys{
		inner: make(map[string]struct{}, len(from)),
	}

	for _, value := range from {
		sk.inner[value] = struct{}{}
	}
	return sk
}

func (sk *StringKeys) Insert(value string) {
	sk.inner[value] = struct{}{}
}

func (sk *StringKeys) ForEach(f func(string)) {
	for value := range sk.inner {
		f(value)
	}
}

func (sk *StringKeys) Keys() []string {
	values := make([]string, 0, len(sk.inner))
	for value := range sk.inner {
		values = append(values, value)
	}
	return values
}

func (sk *StringKeys) MarshalJSON() ([]byte, error) {
	return json.Marshal(sk.Keys())
}

func (sk *StringKeys) MarshalEasyJSON(out *jwriter.Writer) {
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
