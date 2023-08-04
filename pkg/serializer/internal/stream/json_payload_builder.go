// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

//go:build zlib

package stream

import (
	"bytes"
	"expvar"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	jsonStreamExpvars        = expvar.NewMap("jsonstream")
	expvarsTotalCalls        = expvar.Int{}
	expvarsTotalItems        = expvar.Int{}
	expvarsWriteItemErrors   = expvar.Int{}
	expvarsPayloadFulls      = expvar.Int{}
	expvarsItemDrops         = expvar.Int{}
	expvarsCompressorLocks   = expvar.Int{}
	expvarsTotalLockTime     = expvar.Int{}
	expvarsSerializationTime = expvar.Int{}

	tlmTotalCalls             = telemetry.NewCounter("jsonstream", "total_calls", nil, "Total calls to the jsontream serializer")
	tlmTotalItems             = telemetry.NewCounter("jsonstream", "total_items", nil, "Total items in the jsonstream serializer")
	tlmItemDrops              = telemetry.NewCounter("jsonstream", "item_drops", nil, "Items dropped in the jsonstream serializer")
	tlmWriteItemErrors        = telemetry.NewCounter("jsonstream", "write_item_errors", nil, "Count of 'write item errors' in the jsonstream serializer")
	tlmPayloadFull            = telemetry.NewCounter("jsonstream", "payload_full", nil, "How many times we've hit a 'payload is full' in the jsonstream serializer")
	tlmCompressorLocks        = telemetry.NewGauge("jsonstream", "blocking_goroutines", nil, "Number of blocked goroutines waiting for a compressor to be available")
	tlmTotalLockTime          = telemetry.NewCounter("jsonstream", "blocked_time", nil, "Total time spent waiting for the compressor to be available")
	tlmTotalSerializationTime = telemetry.NewCounter("jsonstream", "serialization_time", nil, "Total time spent serializing and compressing payloads")
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
	jsonStreamExpvars.Set("CompressorLocks", &expvarsCompressorLocks)
	jsonStreamExpvars.Set("TotalLockTime", &expvarsTotalLockTime)
	jsonStreamExpvars.Set("TotalSerializationTime", &expvarsSerializationTime)
}

// JSONPayloadBuilder is used to build payloads. JSONPayloadBuilder allocates memory based
// on what was previously need to serialize payloads. Keep that in mind and
// use multiple JSONPayloadBuilders for different sources.
type JSONPayloadBuilder struct {
	inputSizeHint, outputSizeHint int
	shareAndLockBuffers           bool
	input, output                 *bytes.Buffer
	mu                            sync.Mutex
}

// NewJSONPayloadBuilder returns a new JSONPayloadBuilder
func NewJSONPayloadBuilder(shareAndLockBuffers bool) *JSONPayloadBuilder {
	if shareAndLockBuffers {
		return &JSONPayloadBuilder{
			inputSizeHint:       4096,
			outputSizeHint:      4096,
			shareAndLockBuffers: true,
			input:               bytes.NewBuffer(make([]byte, 0, 4096)),
			output:              bytes.NewBuffer(make([]byte, 0, 4096)),
		}
	}
	return &JSONPayloadBuilder{
		inputSizeHint:       4096,
		outputSizeHint:      4096,
		shareAndLockBuffers: false,
	}
}

// OnErrItemTooBigPolicy defines the behavior when OnErrItemTooBig occurs.
type OnErrItemTooBigPolicy int

const (
	// DropItemOnErrItemTooBig skips the error and continues when ErrItemTooBig is encountered
	DropItemOnErrItemTooBig OnErrItemTooBigPolicy = iota

	// FailOnErrItemTooBig returns the error and stop when ErrItemTooBig is encountered
	FailOnErrItemTooBig
)

// BuildWithOnErrItemTooBigPolicy serializes a metadata payload and sends it to the forwarder
func (b *JSONPayloadBuilder) BuildWithOnErrItemTooBigPolicy(
	m marshaler.IterableStreamJSONMarshaler,
	policy OnErrItemTooBigPolicy) (transaction.BytesPayloads, error) {
	var input, output *bytes.Buffer

	// the backend accepts payloads up to specific compressed / uncompressed
	// sizes, but prefers small uncompressed payloads.
	maxPayloadSize := config.Datadog.GetInt("serializer_max_payload_size")
	maxUncompressedSize := config.Datadog.GetInt("serializer_max_uncompressed_payload_size")

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

	var payloads transaction.BytesPayloads
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

	compressor, err := NewCompressor(
		input, output,
		maxPayloadSize, maxUncompressedSize,
		header.Bytes(), footer.Bytes(), []byte(","))
	if err != nil {
		return nil, err
	}

	pointCount := 0
	ok := m.MoveNext()
	for ok {
		// We keep reusing the same small buffer in the jsoniter stream. Note that we can do so
		// because compressor.addItem copies given buffer.
		jsonStream.Reset(nil)
		err := m.WriteCurrentItem(jsonStream)
		if err != nil {
			log.Warnf("error marshalling an item, skipping: %s", err)
			ok = m.MoveNext()
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
			payloads = append(payloads, transaction.NewBytesPayload(payload, pointCount))
			pointCount = 0
			input.Reset()
			output.Reset()
			compressor, err = NewCompressor(
				input, output,
				maxPayloadSize, maxUncompressedSize,
				header.Bytes(), footer.Bytes(), []byte(","))
			if err != nil {
				return nil, err
			}
		case nil:
			// All good, continue to next item
			pointCount += m.GetCurrentItemPointCount()
			ok = m.MoveNext()
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
			log.Warnf("Dropping an item, %s: %s", m.DescribeCurrentItem(), err)
			ok = m.MoveNext()
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
	payloads = append(payloads, transaction.NewBytesPayload(payload, pointCount))

	if !b.shareAndLockBuffers {
		b.inputSizeHint = input.Cap()
		b.outputSizeHint = output.Cap()
	}

	elapsed := time.Since(start)
	expvarsSerializationTime.Add(int64(elapsed))
	tlmTotalSerializationTime.Add(float64(elapsed))

	return payloads, nil
}
