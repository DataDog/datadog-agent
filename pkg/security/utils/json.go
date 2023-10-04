// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"errors"
	"time"

	"github.com/mailru/easyjson/jwriter"
)

// EasyjsonTime represents a EasyJSON enabled time wrapper
type EasyjsonTime struct {
	Inner time.Time
}

// NewEasyjsonTime returns a new EasyjsonTime based on the provided time
func NewEasyjsonTime(t time.Time) EasyjsonTime {
	return EasyjsonTime{Inner: t}
}

// MarshalEasyJSON does JSON marshaling using easyjson interface
func (t EasyjsonTime) MarshalEasyJSON(w *jwriter.Writer) {
	if y := t.Inner.Year(); y < 0 || y >= 10000 {
		if w.Error == nil {
			w.Error = errors.New("Time.MarshalJSON: year outside of range [0,9999]")
		}
		return
	}

	w.Buffer.EnsureSpace(len(time.RFC3339Nano) + 2)
	w.Buffer.AppendByte('"')
	w.Buffer.Buf = t.Inner.AppendFormat(w.Buffer.Buf, time.RFC3339Nano)
	w.Buffer.AppendByte('"')
}

// UnmarshalJSON does JSON unmarshaling
func (t *EasyjsonTime) UnmarshalJSON(b []byte) error {
	return t.Inner.UnmarshalJSON(b)
}
