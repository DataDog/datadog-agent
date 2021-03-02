// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

//+build zlib

package jsonstream

import (
	"bytes"
	"expvar"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmCompressorLocks        = telemetry.NewGauge("jsonstream", "blocking_goroutines", nil, "Number of blocked goroutines waiting for a compressor to be available")
	tlmTotalLockTime          = telemetry.NewCounter("jsonstream", "blocked_time", nil, "Total time spent waiting for the compressor to be available")
	tlmTotalSerializationTime = telemetry.NewCounter("jsonstream", "serialization_time", nil, "Total time spent serializing and compressing payloads")
	expvarsCompressorLocks    = expvar.Int{}
	expvarsTotalLockTime      = expvar.Int{}
	expvarsSerializationTime  = expvar.Int{}
)

var jsonConfig = jsoniter.Config{
	EscapeHTML:                    false,
	ObjectFieldMustBeSimpleString: true,
}.Froze()

// PayloadBuilder is used to build payloads. PayloadBuilder allocates memory based
// on what was previously need to serialize payloads. If shareAndLockBuffers is enabled
// new input/output buffers will be reused and locked to be thread safe. Keep that
// in mind and use multiple PayloadBuilders for different sources.
type PayloadBuilder struct {
	inputSizeHint, outputSizeHint int
	shareAndLockBuffers           bool
	input, output                 *bytes.Buffer
	mu                            sync.Mutex
}

func init() {
	expvars.Set("CompressorLocks", &expvarsCompressorLocks)
	expvars.Set("TotalLockTime", &expvarsTotalLockTime)
	expvars.Set("TotalSerializationTime", &expvarsSerializationTime)
}

// NewPayloadBuilder creates a new PayloadBuilder with default values.
func NewPayloadBuilder(shareAndLockBuffers bool) *PayloadBuilder {
	if shareAndLockBuffers {
		return &PayloadBuilder{
			inputSizeHint:       4096,
			outputSizeHint:      4096,
			shareAndLockBuffers: true,
			input:               bytes.NewBuffer(make([]byte, 0, 4096)),
			output:              bytes.NewBuffer(make([]byte, 0, 4096)),
		}
	}
	return &PayloadBuilder{
		inputSizeHint:       4096,
		outputSizeHint:      4096,
		shareAndLockBuffers: false,
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
func (b *PayloadBuilder) Build(m marshaler.StreamJSONMarshaler) (forwarder.Payloads, error) {
	return b.BuildWithOnErrItemTooBigPolicy(m, DropItemOnErrItemTooBig)
}

// BuildWithOnErrItemTooBigPolicy serializes a metadata payload and sends it to the forwarder
func (b *PayloadBuilder) BuildWithOnErrItemTooBigPolicy(
	m marshaler.StreamJSONMarshaler,
	policy OnErrItemTooBigPolicy) (forwarder.Payloads, error) {

	var input, output *bytes.Buffer
	if b.shareAndLockBuffers {
		defer b.mu.Unlock()

		tlmCompressorLocks.Inc()
		expvarsCompressorLocks.Add(1)
		start := time.Now()
		b.mu.Lock()
		elapsed := time.Since(start)
		expvarsTotalLockTime.Add(int64(elapsed))
		tlmTotalLockTime.Add(float64(elapsed))
		tlmCompressorLocks.Dec()
		expvarsCompressorLocks.Add(-1)

		input = b.input
		output = b.output
		input.Reset()
		output.Reset()
	} else {
		input = bytes.NewBuffer(make([]byte, 0, b.inputSizeHint))
		output = bytes.NewBuffer(make([]byte, 0, b.outputSizeHint))
	}

	var payloads forwarder.Payloads
	var i int
	itemCount := m.Len()
	expvarsTotalCalls.Add(1)
	tlmTotalCalls.Inc()
	start := time.Now()

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

	compressor, err := newCompressor(input, output, header.Bytes(), footer.Bytes())
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

		switch compressor.addItem(jsonStream.Buffer()) {
		case errPayloadFull:
			expvarsPayloadFulls.Add(1)
			tlmPayloadFull.Inc()
			// payload is full, we need to create a new one
			payload, err := compressor.close()
			if err != nil {
				return payloads, err
			}
			payloads = append(payloads, &payload)
			input.Reset()
			output.Reset()
			compressor, err = newCompressor(input, output, header.Bytes(), footer.Bytes())
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
	payload, err := compressor.close()
	if err != nil {
		return payloads, err
	}
	payloads = append(payloads, &payload)

	if !b.shareAndLockBuffers {
		b.inputSizeHint = input.Cap()
		b.outputSizeHint = output.Cap()
	}

	elapsed := time.Since(start)
	expvarsSerializationTime.Add(int64(elapsed))
	tlmTotalSerializationTime.Add(float64(elapsed))

	return payloads, nil
}
