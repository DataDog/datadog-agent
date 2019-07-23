// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

//+build zlib

package jsonstream

import (
	"bytes"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var jsonConfig = jsoniter.Config{
	EscapeHTML:                    false,
	ObjectFieldMustBeSimpleString: true,
}.Froze()

// PayloadBuilder is used to build payloads. PayloadBuilder allocates memory based
// on what was previously need to serialize payloads. Keep that in mind and
// use multiple PayloadBuilders for different sources.
type PayloadBuilder struct {
	inputSizeHint, outputSizeHint int
}

// NewPayloadBuilder creates a new PayloadBuilder with default values.
func NewPayloadBuilder() *PayloadBuilder {
	return &PayloadBuilder{
		inputSizeHint:  4096,
		outputSizeHint: 4096,
	}
}

// Build serializes a metadata payload and sends it to the forwarder
func (b *PayloadBuilder) Build(m marshaler.StreamJSONMarshaler) (forwarder.Payloads, error) {
	var payloads forwarder.Payloads
	var i int
	itemCount := m.Len()
	expvarsTotalCalls.Add(1)

	// Inner buffers for the compressor
	input := bytes.NewBuffer(make([]byte, 0, b.inputSizeHint))
	output := bytes.NewBuffer(make([]byte, 0, b.outputSizeHint))

	// Temporary buffers
	var header, footer bytes.Buffer
	jsonStream := jsoniter.NewStream(jsonConfig, &header, 4096)

	if err := m.Initialize(); err != nil {
		return nil, err
	}

	err := m.WriteHeader(jsonStream)
	if err != nil {
		return nil, err
	}

	jsonStream.Reset(&footer)
	err = m.WriteFooter(jsonStream)
	if err != nil {
		return nil, err
	}
	insertSepBetweenItems := m.SupportJSONSeparatorInsertion()
	compressor, err := newCompressor(input, output, header.Bytes(), footer.Bytes(), insertSepBetweenItems)
	if err != nil {
		return nil, err
	}

	itemIndexInPayload := 0
	for i < itemCount {
		// We keep reusing the same small buffer in the jsoniter stream. Note that we can do so
		// because compressor.addItem copies given buffer.
		jsonStream.Reset(nil)
		err := m.WriteItem(jsonStream, i, itemIndexInPayload)
		if err != nil {
			log.Warnf("error marshalling an item, skipping: %s", err)
			i++
			continue
		}

		switch compressor.addItem(jsonStream.Buffer()) {
		case errPayloadFull:
			// payload is full, we need to create a new one
			payload, err := compressor.close(footer.Bytes())
			if err != nil {
				return payloads, err
			}
			payloads = append(payloads, &payload)
			input.Reset()
			output.Reset()
			compressor, err = newCompressor(input, output, header.Bytes(), footer.Bytes(), insertSepBetweenItems)
			if err != nil {
				return nil, err
			}
			itemIndexInPayload = 0
		case nil:
			// All good, continue to next item
			i++
			itemIndexInPayload++
			expvarsTotalItems.Add(1)
			continue
		default:
			// Unexpected error, drop the item
			i++
			log.Warnf("Dropping an item, %s: %s", m.DescribeItem(i), err)
			expvarsItemDrops.Add(1)
			continue
		}
	}

	// Close last payload
	jsonStream.Reset(nil)
	m.WriteLastFooter(jsonStream, itemIndexInPayload)
	payload, err := compressor.close(jsonStream.Buffer())
	if err != nil {
		return payloads, err
	}
	payloads = append(payloads, &payload)

	b.inputSizeHint = input.Cap()
	b.outputSizeHint = output.Cap()

	return payloads, nil
}
