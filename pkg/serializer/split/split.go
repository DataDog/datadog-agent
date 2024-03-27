// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package split

import (
	"expvar"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// the backend accepts payloads up to 3MB, but being conservative is okay
var maxPayloadSizeCompressed = 2 * 1024 * 1024
var maxPayloadSizeUnCompressed = 64 * 1024 * 1024

// MarshalFct marshal m. Must be either JSONMarshalFct or ProtoMarshalFct.
type MarshalFct func(m marshaler.AbstractMarshaler) ([]byte, error)

// JSONMarshalFct marshal with MarshalJSON method.
func JSONMarshalFct(m marshaler.AbstractMarshaler) ([]byte, error) {
	return (m.(marshaler.JSONMarshaler)).MarshalJSON()
}

// ProtoMarshalFct marshal with Marshal method.
func ProtoMarshalFct(m marshaler.AbstractMarshaler) ([]byte, error) {
	return (m.(marshaler.ProtoMarshaler)).Marshal()
}

var (
	// TODO(remy): could probably be removed as not used in the status page
	splitterExpvars      = expvar.NewMap("splitter")
	splitterNotTooBig    = expvar.Int{}
	splitterTooBig       = expvar.Int{}
	splitterTotalLoops   = expvar.Int{}
	splitterPayloadDrops = expvar.Int{}

	tlmSplitterNotTooBig = telemetry.NewCounter("splitter", "not_too_big",
		nil, "Splitter 'not too big' occurrences")
	tlmSplitterTooBig = telemetry.NewCounter("splitter", "too_big",
		nil, "Splitter 'too big' occurrences")
	tlmSplitterTotalLoops = telemetry.NewCounter("splitter", "total_loops",
		nil, "Splitter total loops run")
	tlmSplitterPayloadDrops = telemetry.NewCounter("splitter", "payload_drops",
		nil, "Splitter payload drops")
)

func init() {
	splitterExpvars.Set("NotTooBig", &splitterNotTooBig)
	splitterExpvars.Set("TooBig", &splitterTooBig)
	splitterExpvars.Set("TotalLoops", &splitterTotalLoops)
	splitterExpvars.Set("PayloadDrops", &splitterPayloadDrops)

}

// CheckSizeAndSerialize Check the size of a payload and marshall it (optionally compress it)
// The dual role makes sense as you will never serialize without checking the size of the payload
func CheckSizeAndSerialize(m marshaler.AbstractMarshaler, compress bool, marshalFct MarshalFct, strategy compression.Component) (bool, []byte, []byte, error) {
	compressedPayload, payload, err := serializeMarshaller(m, compress, marshalFct, strategy)
	if err != nil {
		return false, nil, nil, err
	}

	mustBeSplit := tooBigCompressed(compressedPayload) || tooBigUnCompressed(payload)

	return mustBeSplit, compressedPayload, payload, nil
}

// Payloads serializes a metadata payload and sends it to the forwarder
func Payloads(m marshaler.AbstractMarshaler, compress bool, marshalFct MarshalFct, strategy compression.Component) (transaction.BytesPayloads, error) {
	marshallers := []marshaler.AbstractMarshaler{m}
	smallEnoughPayloads := transaction.BytesPayloads{}
	tooBig, compressedPayload, _, err := CheckSizeAndSerialize(m, compress, marshalFct, strategy)
	if err != nil {
		return smallEnoughPayloads, err
	}
	// If the payload's size is fine, just return it
	if !tooBig {
		log.Debug("The payload was not too big, returning the full payload")
		splitterNotTooBig.Add(1)
		tlmSplitterNotTooBig.Inc()
		smallEnoughPayloads = append(smallEnoughPayloads, transaction.NewBytesPayloadWithoutMetaData(compressedPayload))
		return smallEnoughPayloads, nil
	}
	splitterTooBig.Add(1)
	tlmSplitterTooBig.Inc()
	loops := 0
	// Do not attempt to split payloads forever, if a payload cannot be split then abandon the task
	// the function will return all the payloads that were able to be split
	for tooBig && loops < 3 {
		splitterTotalLoops.Add(1)
		tlmSplitterTotalLoops.Inc()
		// create a temporary slice, the other array will be reused to keep track of the payloads that have yet to be split
		tempSlice := make([]marshaler.AbstractMarshaler, len(marshallers))
		copy(tempSlice, marshallers)
		marshallers = []marshaler.AbstractMarshaler{}
		for _, toSplit := range tempSlice {
			var e error
			// we have to do this every time to get the proper payload
			compressedPayload, payload, e := serializeMarshaller(toSplit, compress, marshalFct, strategy)
			if e != nil {
				return smallEnoughPayloads, e
			}
			payloadSize := len(payload)
			compressedSize := len(compressedPayload)
			// Attempt to account for the compression when estimating the number of chunks that will be needed
			// This is the same function used in dd-agent
			compressionRatio := float64(payloadSize) / float64(compressedSize)
			numChunks := compressedSize/maxPayloadSizeCompressed + 1 + int(compressionRatio/2)
			log.Debugf("split the payload into into %d chunks", numChunks)
			chunks, err := toSplit.SplitPayload(numChunks)
			log.Debugf("payload was split into %d chunks", len(chunks))
			if err != nil {
				log.Warnf("Some payloads could not be split, dropping them")
				splitterPayloadDrops.Add(1)
				tlmSplitterPayloadDrops.Inc()
				return smallEnoughPayloads, err
			}
			// after the payload has been split, loop through the chunks
			for _, chunk := range chunks {
				// serialize the payload
				tooBigChunk, compressedPayload, _, err := CheckSizeAndSerialize(chunk, compress, marshalFct, strategy)
				if err != nil {
					log.Debugf("Error serializing a chunk: %s", err)
					continue
				}
				if !tooBigChunk {
					// if the payload is small enough, return it straight away
					smallEnoughPayloads = append(smallEnoughPayloads, transaction.NewBytesPayloadWithoutMetaData(compressedPayload))
					log.Debugf("chunk was small enough: %v, smallEnoughPayloads are of length: %v", len(compressedPayload), len(smallEnoughPayloads))
				} else {
					// if it is not small enough, append it to the list of payloads
					marshallers = append(marshallers, chunk)
					log.Debugf("chunk was not small enough: %v, marshallers are of length: %v", len(compressedPayload), len(marshallers))
				}
			}
		}
		if len(marshallers) == 0 {
			log.Debug("marshallers was empty, breaking out of the loop")
			tooBig = false
		} else {
			log.Debug("marshallers was not empty, running around the loop again")
			loops++
		}
	}
	if len(marshallers) != 0 {
		log.Warnf("Some payloads could not be split, dropping them")
		splitterPayloadDrops.Add(1)
		tlmSplitterPayloadDrops.Inc()
	}

	return smallEnoughPayloads, nil
}

// serializeMarshaller serializes the marshaller and returns both the compressed and uncompressed payloads
func serializeMarshaller(m marshaler.AbstractMarshaler, compress bool, marshalFct MarshalFct, strategy compression.Component) ([]byte, []byte, error) {
	var payload []byte
	var compressedPayload []byte
	var err error
	payload, err = marshalFct(m)
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

// GetPayloadDrops returns the number of times we dropped some payloads because we couldn't split them.
func GetPayloadDrops() int64 {
	return splitterPayloadDrops.Value()
}
