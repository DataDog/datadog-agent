// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package stream

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// Build serializes a metadata payload and sends it to the forwarder
func BuildJSONPayload(b *JSONPayloadBuilder, m marshaler.StreamJSONMarshaler) (transaction.BytesPayloads, error) {
	adapter := marshaler.NewIterableStreamJSONMarshalerAdapter(m)
	return b.BuildWithOnErrItemTooBigPolicy(adapter, DropItemOnErrItemTooBig)
}
