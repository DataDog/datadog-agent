// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshaler

import (
	jsoniter "github.com/json-iterator/go"
)

var _ IterableStreamJSONMarshaler = (*IterableStreamJSONMarshalerAdapter)(nil)

// IterableStreamJSONMarshalerAdapter adapts an object implementing `StreamJSONMarshaler`
// into an object implementing `IterableStreamJSONMarshaler`
type IterableStreamJSONMarshalerAdapter struct {
	marshaler StreamJSONMarshaler
	index     int
}

// NewIterableStreamJSONMarshalerAdapter creates an new instance of `IterableStreamJSONMarshalerAdapter`
func NewIterableStreamJSONMarshalerAdapter(marshaler StreamJSONMarshaler) *IterableStreamJSONMarshalerAdapter {
	panic("not called")
}

// WriteHeader writes the payload header for this type
func (a *IterableStreamJSONMarshalerAdapter) WriteHeader(j *jsoniter.Stream) error {
	panic("not called")
}

// WriteFooter writes the payload footer for this type
func (a *IterableStreamJSONMarshalerAdapter) WriteFooter(j *jsoniter.Stream) error {
	panic("not called")
}

// WriteCurrentItem writes the json representation into the stream
func (a *IterableStreamJSONMarshalerAdapter) WriteCurrentItem(j *jsoniter.Stream) error {
	panic("not called")
}

// DescribeCurrentItem returns a text description
func (a *IterableStreamJSONMarshalerAdapter) DescribeCurrentItem() string {
	panic("not called")
}

// MoveNext moves to the next value. Returns false when reaching the end of the iteration.
func (a *IterableStreamJSONMarshalerAdapter) MoveNext() bool {
	panic("not called")
}

// GetCurrentItemPointCount gets the number of points in the current item
func (a *IterableStreamJSONMarshalerAdapter) GetCurrentItemPointCount() int {
	panic("not called")
}
