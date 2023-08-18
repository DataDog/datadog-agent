// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

//go:build !zlib

package stream

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// OnErrItemTooBigPolicy defines the behavior when OnErrItemTooBig occurs.
type OnErrItemTooBigPolicy int

const (
	// DropItemOnErrItemTooBig when founding an ErrItemTooBig, skips the error and continue
	DropItemOnErrItemTooBig OnErrItemTooBigPolicy = iota

	// FailOnErrItemTooBig when founding an ErrItemTooBig, returns the error and stop
	FailOnErrItemTooBig
)

// JSONPayloadBuilder is not implemented when zlib is not available.
type JSONPayloadBuilder struct {
}

// NewJSONPayloadBuilder is not implemented when zlib is not available.
func NewJSONPayloadBuilder(shareAndLockBuffers bool) *JSONPayloadBuilder {
	return nil
}

// BuildWithOnErrItemTooBigPolicy is not implemented when zlib is not available.
func (b *JSONPayloadBuilder) BuildWithOnErrItemTooBigPolicy(marshaler.IterableStreamJSONMarshaler, OnErrItemTooBigPolicy) (transaction.BytesPayloads, error) {
	return nil, fmt.Errorf("not implemented")
}
