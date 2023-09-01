// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

// Package serializers defines functions aiming to serialize events
package serializers

import (
	jlexer "github.com/mailru/easyjson/jlexer"
	jwriter "github.com/mailru/easyjson/jwriter"
)

// EventSerializer serializes an event to JSON
type EventSerializer struct{}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v EventSerializer) MarshalEasyJSON(w *jwriter.Writer) {
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *EventSerializer) UnmarshalEasyJSON(l *jlexer.Lexer) {
}
