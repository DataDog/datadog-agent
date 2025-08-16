// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package split

import (
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// the backend accepts payloads up to 3MB, but being conservative is okay
var maxPayloadSizeCompressed = 2 * 1024 * 1024
var maxPayloadSizeUnCompressed = 64 * 1024 * 1024

// CheckSizeAndSerialize Check the size of a payload and marshall it (optionally compress it)
// The dual role makes sense as you will never serialize without checking the size of the payload
func CheckSizeAndSerialize(m marshaler.JSONMarshaler, compress bool, strategy compression.Component) (bool, []byte, []byte, error) {
	compressedPayload, payload, err := serializeMarshaller(m, compress, strategy)
	if err != nil {
		return false, nil, nil, err
	}

	mustBeSplit := tooBigCompressed(compressedPayload) || tooBigUnCompressed(payload)

	return mustBeSplit, compressedPayload, payload, nil
}

// serializeMarshaller serializes the marshaller and returns both the compressed and uncompressed payloads
func serializeMarshaller(m marshaler.AbstractMarshaler, compress bool, strategy compression.Component) ([]byte, []byte, error) {
	var payload []byte
	var compressedPayload []byte
	var err error
	payload, err = m.MarshalJSON()
	compressedPayload = payload
	if err != nil {
		return nil, nil, err
	}
	if compress {
		compressedPayload, err = strategy.Compress(payload)
		if err != nil {
			return nil, nil, err
		}
	}
	return compressedPayload, payload, nil
}

// returns true if the payload is above the max compressed size limit
func tooBigCompressed(payload []byte) bool {
	return len(payload) > maxPayloadSizeCompressed
}

// returns true if the payload is above the max unCompressed size limit
func tooBigUnCompressed(payload []byte) bool {
	return len(payload) > maxPayloadSizeUnCompressed
}
