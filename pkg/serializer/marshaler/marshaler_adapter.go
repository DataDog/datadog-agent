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
	return &IterableStreamJSONMarshalerAdapter{
		marshaler: marshaler,
		index:     -1,
	}
}

// WriteHeader writes the payload header for this type
func (a *IterableStreamJSONMarshalerAdapter) WriteHeader(j *jsoniter.Stream) error {
	return a.marshaler.WriteHeader(j)
}

// WriteFooter writes the payload footer for this type
func (a *IterableStreamJSONMarshalerAdapter) WriteFooter(j *jsoniter.Stream) error {
	return a.marshaler.WriteFooter(j)
}

// WriteCurrentItem writes the json representation into the stream
func (a *IterableStreamJSONMarshalerAdapter) WriteCurrentItem(j *jsoniter.Stream) error {
	return a.marshaler.WriteItem(j, a.index)
}

// DescribeCurrentItem returns a text description
func (a *IterableStreamJSONMarshalerAdapter) DescribeCurrentItem() string {
	return a.marshaler.DescribeItem(a.index)
}

// MoveNext moves to the next value. Returns false when reaching the end of the iteration.
func (a *IterableStreamJSONMarshalerAdapter) MoveNext() bool {
	a.index++
	return a.index < a.marshaler.Len()
}

// GetCurrentItemPointCount gets the number of points in the current item
func (a *IterableStreamJSONMarshalerAdapter) GetCurrentItemPointCount() int {
	return 0
}
