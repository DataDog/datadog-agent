// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package marshaler

import (
	"errors"
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

// DummyMarshaller implements StreamJSONMarshaler for unit tests.
type DummyMarshaller struct {
	Items  []string
	Header string
	Footer string
}

// WriteHeader writes the payload header for this type
func (d *DummyMarshaller) WriteHeader(stream *jsoniter.Stream) error {
	_, err := stream.Write([]byte(d.Header))
	return err
}

// Len returns the number of items to marshal
func (d *DummyMarshaller) Len() int {
	return len(d.Items)
}

// WriteItem writes the json representation of an item
func (d *DummyMarshaller) WriteItem(stream *jsoniter.Stream, i int) error {
	if i < 0 || i > d.Len()-1 {
		return errors.New("out of range")
	}
	_, err := stream.Write([]byte(d.Items[i]))
	return err
}

// DescribeItem returns a text description for logs
func (d *DummyMarshaller) DescribeItem(i int) string {
	if i < 0 || i > d.Len()-1 {
		return "out of range"
	}
	return d.Items[i]
}

// WriteFooter writes the payload footer for this type
func (d *DummyMarshaller) WriteFooter(stream *jsoniter.Stream) error {
	_, err := stream.Write([]byte(d.Footer))
	return err
}

// MarshalJSON not implemented
func (d *DummyMarshaller) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

// Marshal not implemented
func (d *DummyMarshaller) Marshal() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}
