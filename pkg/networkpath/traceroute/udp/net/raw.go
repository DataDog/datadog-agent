// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Contains BSD-2-Clause code (c) 2015-present Andrea Barberio

package net

// Raw is a raw payload
type Raw struct {
	Data []byte
}

// NewRaw builds a new Raw layer
func NewRaw(b []byte) (*Raw, error) {
	var r Raw
	if err := r.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return &r, nil
}

// Next returns the next layer. For Raw the next layer is always nil
func (r Raw) Next() Layer {
	return nil
}

// SetNext sets the next layer. For Raw this is a no op
func (r Raw) SetNext(Layer) {}

// MarshalBinary serializes the layer
func (r Raw) MarshalBinary() ([]byte, error) {
	return r.Data, nil
}

// UnmarshalBinary deserializes the layer
func (r *Raw) UnmarshalBinary(b []byte) error {
	r.Data = b
	return nil
}
