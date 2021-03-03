// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

//+build zlib

package stream

import (
	"bytes"
	"expvar"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// TODO(remy): could probably removed as not appearing in the status page
	jsonStreamExpvars      = expvar.NewMap("jsonstream")
	expvarsTotalCalls      = expvar.Int{}
	expvarsTotalItems      = expvar.Int{}
	expvarsWriteItemErrors = expvar.Int{}
	expvarsPayloadFulls    = expvar.Int{}
	expvarsItemDrops       = expvar.Int{}

	tlmTotalCalls = telemetry.NewCounter("jsonstream", "total_calls",
		nil, "Total calls to the jsontream serializer")
	tlmTotalItems = telemetry.NewCounter("jsonstream", "total_items",
		nil, "Total items in the jsonstream serializer")
	tlmItemDrops = telemetry.NewCounter("jsonstream", "item_drops",
		nil, "Items dropped in the jsonstream serializer")
	tlmWriteItemErrors = telemetry.NewCounter("jsonstream", "write_item_errors",
		nil, "Count of 'write item errors' in the jsonstream serializer")
	tlmPayloadFull = telemetry.NewCounter("jsonstream", "payload_full",
		nil, "How many times we've hit a 'payload is full' in the jsonstream serializer")
)

var jsonConfig = jsoniter.Config{
	EscapeHTML:                    false,
	ObjectFieldMustBeSimpleString: true,
}.Froze()

func init() {
	jsonStreamExpvars.Set("TotalCalls", &expvarsTotalCalls)
	jsonStreamExpvars.Set("TotalItems", &expvarsTotalItems)
	jsonStreamExpvars.Set("WriteItemErrors", &expvarsWriteItemErrors)
	jsonStreamExpvars.Set("PayloadFulls", &expvarsPayloadFulls)
	jsonStreamExpvars.Set("ItemDrops", &expvarsItemDrops)
}

// JSONPayloadBuilder is used to build payloads. JSONPayloadBuilder allocates memory based
// on what was previously need to serialize payloads. Keep that in mind and
// use multiple JSONPayloadBuilders for different sources.
type JSONPayloadBuilder struct {
	inputSizeHint, outputSizeHint int
}

// NewJSONPayloadBuilder creates a new JSONPayloadBuilder with default values.
func NewJSONPayloadBuilder() *JSONPayloadBuilder {
	return &JSONPayloadBuilder{
		inputSizeHint:  4096,
		outputSizeHint: 4096,
	}
}

// OnErrItemTooBigPolicy defines the behavior when OnErrItemTooBig occurs.
type OnErrItemTooBigPolicy int

const (
	// DropItemOnErrItemTooBig:  when ErrItemTooBig is encountered, skips the error and continue
	DropItemOnErrItemTooBig OnErrItemTooBigPolicy = iota

	// FailOnErrItemTooBig: when ErrItemTooBig is encountered, returns the error and stop
	FailOnErrItemTooBig
)

// Build serializes a metadata payload and sends it to the forwarder
func (b *JSONPayloadBuilder) Build(m marshaler.StreamJSONMarshaler) (forwarder.Payloads, error) {
	return b.BuildWithOnErrItemTooBigPolicy(m, DropItemOnErrItemTooBig)
}

// BuildWithOnErrItemTooBigPolicy serializes a metadata payload and sends it to the forwarder
func (b *JSONPayloadBuilder) BuildWithOnErrItemTooBigPolicy(
	m marshaler.StreamJSONMarshaler,
	policy OnErrItemTooBigPolicy) (forwarder.Payloads, error) {

	var payloads forwarder.Payloads
	var i int
	itemCount := m.Len()
	expvarsTotalCalls.Add(1)
	tlmTotalCalls.Inc()

	// Inner buffers for the compressor
	input := bytes.NewBuffer(make([]byte, 0, b.inputSizeHint))
	output := bytes.NewBuffer(make([]byte, 0, b.outputSizeHint))

	// Temporary buffers
	var header, footer bytes.Buffer
	jsonStream := jsoniter.NewStream(jsonConfig, &header, 4096)

	err := m.WriteHeader(jsonStream)
	if err != nil {
		return nil, err
	}

	jsonStream.Reset(&footer)
	err = m.WriteFooter(jsonStream)
	if err != nil {
		return nil, err
	}

	compressor, err := NewCompressor(input, output, header.Bytes(), footer.Bytes(), []byte(","))
	if err != nil {
		return nil, err
	}

	for i < itemCount {
		// We keep reusing the same small buffer in the jsoniter stream. Note that we can do so
		// because compressor.addItem copies given buffer.
		jsonStream.Reset(nil)
		err := m.WriteItem(jsonStream, i)
		if err != nil {
			log.Warnf("error marshalling an item, skipping: %s", err)
			i++
			expvarsWriteItemErrors.Add(1)
			tlmWriteItemErrors.Inc()
			continue
		}

		switch compressor.AddItem(jsonStream.Buffer()) {
		case ErrPayloadFull:
			expvarsPayloadFulls.Add(1)
			tlmPayloadFull.Inc()
			// payload is full, we need to create a new one
			payload, err := compressor.Close()
			if err != nil {
				return payloads, err
			}
			payloads = append(payloads, &payload)
			input.Reset()
			output.Reset()
			compressor, err = NewCompressor(input, output, header.Bytes(), footer.Bytes(), []byte(","))
			if err != nil {
				return nil, err
			}
		case nil:
			// All good, continue to next item
			i++
			expvarsTotalItems.Add(1)
			tlmTotalItems.Inc()
			continue
		case ErrItemTooBig:
			if policy == FailOnErrItemTooBig {
				return nil, ErrItemTooBig
			}
			fallthrough
		default:
			// Unexpected error, drop the item
			i++
			log.Warnf("Dropping an item, %s: %s", m.DescribeItem(i), err)
			expvarsItemDrops.Add(1)
			tlmItemDrops.Inc()
			continue
		}
	}

	// Close last payload
	payload, err := compressor.Close()
	if err != nil {
		return payloads, err
	}
	payloads = append(payloads, &payload)

	b.inputSizeHint = input.Cap()
	b.outputSizeHint = output.Cap()

	return payloads, nil
}
