// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"errors"
	"time"

	"github.com/mailru/easyjson"
	"github.com/mailru/easyjson/jwriter"
)

// EasyjsonTime represents a EasyJSON enabled time wrapper
type EasyjsonTime struct {
	inner time.Time
}

// NewEasyjsonTime returns a new EasyjsonTime based on the provided time
func NewEasyjsonTime(t time.Time) EasyjsonTime {
	return EasyjsonTime{inner: t}
}

// NewEasyjsonTimeIfNotZero returns a new EasyjsonTime based on the provided time or nil if zero time
func NewEasyjsonTimeIfNotZero(t time.Time) *EasyjsonTime {
	if t.IsZero() {
		return nil
	}
	tt := NewEasyjsonTime(t)
	return &tt
}

// MarshalEasyJSON does JSON marshaling using easyjson interface
func (t EasyjsonTime) MarshalEasyJSON(w *jwriter.Writer) {
	if y := t.inner.Year(); y < 0 || y >= 10000 {
		if w.Error == nil {
			w.Error = errors.New("Time.MarshalJSON: year outside of range [0,9999]")
		}
		return
	}

	w.Buffer.EnsureSpace(len(time.RFC3339Nano) + 2)
	w.Buffer.AppendByte('"')
	w.Buffer.Buf = t.inner.AppendFormat(w.Buffer.Buf, time.RFC3339Nano)
	w.Buffer.AppendByte('"')
}

// UnmarshalJSON does JSON unmarshaling
func (t *EasyjsonTime) UnmarshalJSON(b []byte) error {
	return t.inner.UnmarshalJSON(b)
}

// GetInnerTime returns the inner time
func (t *EasyjsonTime) GetInnerTime() time.Time {
	return t.inner
}

// MarshalEasyJSON easyjson marshal helper
func MarshalEasyJSON(i easyjson.Marshaler) ([]byte, error) {
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	i.MarshalEasyJSON(w)
	return w.BuildBytes()
}
