// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"bytes"
	"expvar"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/richardartoul/molecule"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// A SketchSeriesList implements marshaler.Marshaler
type SketchSeriesList struct {
	metrics.SketchesSource
}

var (
	expvars                    = expvar.NewMap("sketch_series")
	expvarsItemTooBig          = expvar.Int{}
	expvarsPayloadFull         = expvar.Int{}
	expvarsUnexpectedItemDrops = expvar.Int{}
	tlmItemTooBig              = telemetry.NewCounter("sketch_series", "sketch_too_big",
		nil, "Number of payloads dropped because they were too big for the stream compressor")
	tlmPayloadFull = telemetry.NewCounter("sketch_series", "payload_full",
		nil, "How many times we've hit a 'payload is full' in the stream compressor")
	tlmUnexpectedItemDrops = telemetry.NewCounter("sketch_series", "unexpected_item_drops",
		nil, "Items dropped in the stream compressor")
)

func init() {
	expvars.Set("ItemTooBig", &expvarsItemTooBig)
	expvars.Set("PayloadFull", &expvarsPayloadFull)
	expvars.Set("UnexpectedItemDrops", &expvarsUnexpectedItemDrops)
}

// MarshalSplitCompress uses the stream compressor to marshal and compress sketch series payloads.
// If a compressed payload is larger than the max, a new payload will be generated. This method returns a slice of
// compressed protobuf marshaled gogen.SketchPayload objects. gogen.SketchPayload is not directly marshaled - instead
// it's contents are marshaled individually, packed with the appropriate protobuf metadata, and compressed in stream.
// The resulting payloads (when decompressed) are binary equal to the result of marshaling the whole object at once.
func (sl SketchSeriesList) MarshalSplitCompress(bufferContext *marshaler.BufferContext, config config.Component, strategy compression.Component) (transaction.BytesPayloads, error) {
	var err error

	pb := newPayloadsBuilder(bufferContext, config, strategy)

	// start things off
	err = pb.startPayload()
	if err != nil {
		return nil, err
	}

	for sl.MoveNext() {
		ss := sl.Current()
		err = pb.marshal(ss)
		if err != nil {
			return nil, err
		}
	}

	err = pb.finishPayload()
	if err != nil {
		log.Debugf("Failed to finish payload with err %v", err)
		return nil, err
	}

	return pb.payloads, nil
}

// MarshalSplitCompressMultiple uses the stream compressor to marshal and
// compress one sketch list into two sets of payloads.  One set of payloads
// contains all metrics, and the other contains only those that pass the
// provided filter function.  This function exists because we need a way to
// build both payloads in a single pass over the input data, which cannot be
// iterated over twice.
func (sl SketchSeriesList) MarshalSplitCompressMultiple(config config.Component, strategy compression.Component, filterFunc func(ss *metrics.SketchSeries) bool) (transaction.BytesPayloads, transaction.BytesPayloads, error) {
	var err error

	bufferContext := marshaler.NewBufferContext()
	bufferContext2 := marshaler.NewBufferContext()

	pb := newPayloadsBuilder(bufferContext, config, strategy)
	pb2 := newPayloadsBuilder(bufferContext2, config, strategy)

	// start things off
	err = pb.startPayload()
	if err != nil {
		return nil, nil, err
	}
	err = pb2.startPayload()
	if err != nil {
		return nil, nil, err
	}

	for sl.MoveNext() {
		ss := sl.Current()
		err = pb.marshal(ss)
		if err != nil {
			return nil, nil, err
		}
		if filterFunc(ss) {
			err = pb2.marshal(ss)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	err = pb.finishPayload()
	if err != nil {
		log.Debugf("Failed to finish payload with err %v", err)
		return nil, nil, err
	}

	err = pb2.finishPayload()
	if err != nil {
		log.Debugf("Failed to finish payload with err %v", err)
		return nil, nil, err
	}

	return pb.payloads, pb2.payloads, nil
}

func newPayloadsBuilder(bufferContext *marshaler.BufferContext, config config.Component, strategy compression.Component) payloadsBuilder {
	buf := bufferContext.PrecompressionBuf
	pb := payloadsBuilder{
		bufferContext: bufferContext,
		strategy:      strategy,
		compressor:    nil,
		buf:           buf,
		payloads:      transaction.BytesPayloads{},
		ps:            molecule.NewProtoStream(buf),
		// the backend accepts payloads up to specific compressed / uncompressed
		// sizes, but prefers small uncompressed payloads.
		maxPayloadSize:      config.GetInt("serializer_max_payload_size"),
		maxUncompressedSize: config.GetInt("serializer_max_uncompressed_payload_size"),
		pointCount:          0,
	}
	return pb
}

type payloadsBuilder struct {
	bufferContext       *marshaler.BufferContext
	strategy            compression.Component
	compressor          *stream.Compressor
	buf                 *bytes.Buffer
	payloads            transaction.BytesPayloads
	ps                  *molecule.ProtoStream
	maxPayloadSize      int
	maxUncompressedSize int
	pointCount          int
}

// Prepare to write the next payload
func (pb *payloadsBuilder) startPayload() error {
	// constants for the protobuf data we will be writing, taken from
	// https://github.com/DataDog/agent-payload/v5/blob/a2cd634bc9c088865b75c6410335270e6d780416/proto/metrics/agent_payload.proto#L47-L81
	// Unused fields are commented out
	// const payloadSketches = 1
	const payloadMetadata = 2

	// Generate a footer containing an empty Metadata field.  The gogoproto
	// generated serialization code includes this when marshaling the struct,
	// despite the protobuf encoding not really requiring it (all fields
	// default to their zero value)
	var footer []byte
	{
		buf := bytes.NewBuffer([]byte{})
		ps := molecule.NewProtoStream(buf)
		_ = ps.Embedded(payloadMetadata, func(_ *molecule.ProtoStream) error {
			return nil
		})
		footer = buf.Bytes()
	}

	pb.bufferContext.CompressorInput.Reset()
	pb.bufferContext.CompressorOutput.Reset()
	pb.pointCount = 0
	compressor, err := stream.NewCompressor(
		pb.bufferContext.CompressorInput, pb.bufferContext.CompressorOutput,
		pb.maxPayloadSize, pb.maxUncompressedSize,
		[]byte{}, footer, []byte{}, pb.strategy)
	if err != nil {
		return err
	}

	pb.compressor = compressor

	return nil
}

func (pb *payloadsBuilder) marshal(ss *metrics.SketchSeries) error {
	// constants for the protobuf data we will be writing, taken from
	// https://github.com/DataDog/agent-payload/v5/blob/a2cd634bc9c088865b75c6410335270e6d780416/proto/metrics/agent_payload.proto#L47-L81
	// Unused fields are commented out
	const payloadSketches = 1
	// const payloadMetadata = 2
	const sketchMetric = 1
	const sketchHost = 2
	// const sketchDistributions = 3
	const sketchTags = 4
	const sketchDogsketches = 7
	const sketchMetadata = 8
	// const distributionTs = 1
	// const distributionCnt = 2
	// const distributionMin = 3
	// const distributionMax = 4
	// const distributionAvg = 5
	// const distributionSum = 6
	// const distributionV = 7
	// const distributionG = 8
	// const distributionDelta = 9
	// const distributionBuf = 10
	const dogsketchTs = 1
	const dogsketchCnt = 2
	const dogsketchMin = 3
	const dogsketchMax = 4
	const dogsketchAvg = 5
	const dogsketchSum = 6
	const dogsketchK = 7
	const dogsketchN = 8

	const sketchMetadataOrigin = 1
	//         |------| 'Metadata' message
	//                 |-----| 'origin' field index
	const sketchMetadataOriginMetricType = 3
	//         |------| 'Metadata' message
	//                 |----| 'origin' message
	//                       |--------| 'metric_type' field index
	const metryTypeNotIndexed = 9
	//    |-----------------| 'metric_type_agent_hidden' field index

	const sketchMetadataOriginOriginProduct = 4
	//                 |----|  'Origin' message
	//                       |-----------| 'origin_product' field index
	const sketchMetadataOriginOriginCategory = 5
	//                 |----|  'Origin' message
	//                       |-----------| 'origin_category' field index
	const sketchMetadataOriginOriginService = 6
	//                 |----|  'Origin' message
	//                       |-----------| 'origin_service' field index
	const serieMetadataOriginOriginProductAgentType = 10
	//                 |----|  'Origin' message
	//                       |-----------| 'OriginProduct' enum
	//                                    |-------| 'Agent' enum value

	pb.buf.Reset()
	err := pb.ps.Embedded(payloadSketches, func(ps *molecule.ProtoStream) error {
		var err error

		err = ps.String(sketchMetric, ss.Name)
		if err != nil {
			return err
		}

		err = ps.String(sketchHost, ss.Host)
		if err != nil {
			return err
		}

		err = ss.Tags.ForEachErr(func(tag string) error {
			return ps.String(sketchTags, tag)
		})
		if err != nil {
			return err
		}

		for _, p := range ss.Points {
			err = ps.Embedded(sketchDogsketches, func(ps *molecule.ProtoStream) error {
				b := p.Sketch.Basic
				k, n := p.Sketch.Cols()

				err = ps.Int64(dogsketchTs, p.Ts)
				if err != nil {
					return err
				}

				err = ps.Int64(dogsketchCnt, b.Cnt)
				if err != nil {
					return err
				}

				err = ps.Double(dogsketchMin, b.Min)
				if err != nil {
					return err
				}

				err = ps.Double(dogsketchMax, b.Max)
				if err != nil {
					return err
				}

				err = ps.Double(dogsketchAvg, b.Avg)
				if err != nil {
					return err
				}

				err = ps.Double(dogsketchSum, b.Sum)
				if err != nil {
					return err
				}

				err = ps.Sint32Packed(dogsketchK, k)
				if err != nil {
					return err
				}

				err = ps.Uint32Packed(dogsketchN, n)
				if err != nil {
					return err
				}

				return nil
			})
			if err != nil {
				return err
			}
		}
		err = ps.Embedded(sketchMetadata, func(ps *molecule.ProtoStream) error {
			return ps.Embedded(sketchMetadataOrigin, func(ps *molecule.ProtoStream) error {
				if ss.NoIndex {
					err = ps.Int32(sketchMetadataOriginMetricType, metryTypeNotIndexed)
					if err != nil {
						return err
					}
				}
				err = ps.Int32(sketchMetadataOriginOriginProduct, serieMetadataOriginOriginProductAgentType)
				if err != nil {
					return err
				}
				err = ps.Int32(sketchMetadataOriginOriginCategory, metricSourceToOriginCategory(ss.Source))
				if err != nil {
					return err
				}
				return ps.Int32(sketchMetadataOriginOriginService, metricSourceToOriginService(ss.Source))
			})
		})
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Compress the protobuf metadata and the marshaled sketch
	err = pb.compressor.AddItem(pb.buf.Bytes())
	switch err {
	case stream.ErrPayloadFull:
		expvarsPayloadFull.Add(1)
		tlmPayloadFull.Inc()

		// Since the compression buffer is full - flush it and start a new one
		err = pb.finishPayload()
		if err != nil {
			return err
		}

		err = pb.startPayload()
		if err != nil {
			return err
		}

		// Add it to the new compression buffer
		err = pb.compressor.AddItem(pb.buf.Bytes())
		if err == stream.ErrItemTooBig {
			// Item was too big, drop it
			expvarsItemTooBig.Add(1)
			tlmItemTooBig.Inc()
			return nil
		}
		if err != nil {
			// Unexpected error bail out
			expvarsUnexpectedItemDrops.Add(1)
			tlmUnexpectedItemDrops.Inc()
			log.Debugf("Unexpected error trying to addItem to new payload after previous payload filled up: %v", err)
			return err
		}
		pb.pointCount += len(ss.Points)
	case stream.ErrItemTooBig:
		// Item was too big, drop it
		expvarsItemTooBig.Add(1)
		tlmItemTooBig.Add(1)
	case nil:
		pb.pointCount += len(ss.Points)
		return nil
	default:
		// Unexpected error bail out
		expvarsUnexpectedItemDrops.Add(1)
		tlmUnexpectedItemDrops.Inc()
		log.Debugf("Unexpected error: %v", err)
		return err
	}

	return nil
}

func (pb *payloadsBuilder) finishPayload() error {
	payload, err := pb.compressor.Close()
	if err != nil {
		return err
	}

	pb.payloads = append(pb.payloads, transaction.NewBytesPayload(payload, pb.pointCount))

	return nil
}

// Marshal encodes this series list.
func (sl SketchSeriesList) Marshal() ([]byte, error) {
	pb := &gogen.SketchPayload{
		Sketches: make([]gogen.SketchPayload_Sketch, 0),
	}

	for sl.MoveNext() {
		ss := sl.Current()
		dsl := make([]gogen.SketchPayload_Sketch_Dogsketch, 0, len(ss.Points))

		for _, p := range ss.Points {
			b := p.Sketch.Basic
			k, n := p.Sketch.Cols()
			dsl = append(dsl, gogen.SketchPayload_Sketch_Dogsketch{
				Ts:  p.Ts,
				Cnt: b.Cnt,
				Min: b.Min,
				Max: b.Max,
				Avg: b.Avg,
				Sum: b.Sum,
				K:   k,
				N:   n,
			})
		}

		pb.Sketches = append(pb.Sketches, gogen.SketchPayload_Sketch{
			Metric:      ss.Name,
			Host:        ss.Host,
			Tags:        ss.Tags.UnsafeToReadOnlySliceString(),
			Dogsketches: dsl,
		})
	}
	return pb.Marshal()
}

// SplitPayload breaks the payload into times number of pieces
func (sl SketchSeriesList) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	var sketches SketchSeriesSlice
	for sl.MoveNext() {
		ss := sl.Current()
		sketches = append(sketches, ss)
	}
	if len(sketches) == 0 {
		return []marshaler.AbstractMarshaler{}, nil
	}
	return sketches.SplitPayload(times)
}

//nolint:revive // TODO(AML) Fix revive linter
type SketchSeriesSlice []*metrics.SketchSeries

// SplitPayload breaks the payload into times number of pieces
func (sl SketchSeriesSlice) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	// Only break it down as much as possible
	if len(sl) < times {
		times = len(sl)
	}
	splitPayloads := make([]marshaler.AbstractMarshaler, times)
	batchSize := len(sl) / times
	n := 0
	for i := 0; i < times; i++ {
		var end int
		// In many cases the batchSize is not perfect
		// so the last one will be a bit bigger or smaller than the others
		if i < times-1 {
			end = n + batchSize
		} else {
			end = len(sl)
		}
		newSL := sl[n:end]
		splitPayloads[i] = newSL
		n += batchSize
	}
	return splitPayloads, nil
}
