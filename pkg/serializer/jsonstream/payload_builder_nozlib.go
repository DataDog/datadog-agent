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

// OnErrTooBigPolicy defines the behavior when OnErrTooBig occurs.
type OnErrTooBigPolicy int

const (
	// ContinueOnErrTooBig when founding an errTooBig, skips the error and continue
	ContinueOnErrTooBig OnErrTooBigPolicy = iota

	// FailedErrTooBig when founding an errTooBig, returns the error and stop
	FailedErrTooBig
)

// PayloadBuilder is not implemented when zlib is not available.
type PayloadBuilder struct {
}

// NewPayloadBuilder is not implemented when zlib is not available.
func NewPayloadBuilder() *PayloadBuilder {
	return nil
}

// BuildWithOnErrTooBigPolicy is not implemented when zlib is not available.
func (b *PayloadBuilder) BuildWithOnErrTooBigPolicy(marshaler.StreamJSONMarshaler, OnErrTooBigPolicy) (forwarder.Payloads, error) {
	return nil, fmt.Errorf("not implemented")
}
