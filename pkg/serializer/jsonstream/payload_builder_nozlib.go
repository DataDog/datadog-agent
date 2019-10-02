// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

//+build !zlib

package jsonstream

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// OnErrItemTooBigPolicy defines the behavior when OnErrItemTooBig occurs.
type OnErrItemTooBigPolicy int

const (
	// ContinueOnErrItemTooBig when founding an ErrItemTooBig, skips the error and continue
	ContinueOnErrItemTooBig OnErrItemTooBigPolicy = iota

	// FailedErrItemTooBig when founding an ErrItemTooBig, returns the error and stop
	FailedErrItemTooBig
)

// PayloadBuilder is not implemented when zlib is not available.
type PayloadBuilder struct {
}

// NewPayloadBuilder is not implemented when zlib is not available.
func NewPayloadBuilder() *PayloadBuilder {
	return nil
}

// BuildWithOnErrItemTooBigPolicy is not implemented when zlib is not available.
func (b *PayloadBuilder) BuildWithOnErrItemTooBigPolicy(marshaler.StreamJSONMarshaler, OnErrItemTooBigPolicy) (forwarder.Payloads, error) {
	return nil, fmt.Errorf("not implemented")
}
