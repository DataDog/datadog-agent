// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"encoding/binary"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/config"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	statefulgrpc "github.com/DataDog/datadog-agent/pkg/serializer/grpc"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	pkgcompression "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
)

// Per-datum-kind telemetry (count and bytes emitted by the encoder). Declared
// here rather than in the grpc package so the encoder doesn't import gRPC deps.
var (
	tlmStatefulDatumCount = telemetryimpl.GetCompatComponent().NewCounter(
		"serializer", "v3_stateful_datum_count",
		[]string{"kind"},
		"Count of metric datum emitted by the stateful v3 encoder, by kind",
	)
	tlmStatefulDatumBytes = telemetryimpl.GetCompatComponent().NewCounter(
		"serializer", "v3_stateful_datum_bytes",
		[]string{"kind"},
		"Bytes of metric datum emitted by the stateful v3 encoder, by kind (proto.Size)",
	)
)

// datumKindName returns the telemetry-kind label for a MetricDatum, or
// "unknown" for an unrecognized oneof type.
func datumKindName(d *statefulpb.MetricDatum) string {
	switch d.Data.(type) {
	case *statefulpb.MetricDatum_MetricNameDefine:
		return "name_define"
	case *statefulpb.MetricDatum_MetricTagStringDefine:
		return "tag_string_define"
	case *statefulpb.MetricDatum_MetricSourceTypeNameDefine:
		return "source_type_name_define"
	case *statefulpb.MetricDatum_MetricResourceStringDefine:
		return "resource_string_define"
	case *statefulpb.MetricDatum_MetricResourceDefine:
		return "resource_define"
	case *statefulpb.MetricDatum_MetricOriginDefine:
		return "origin_define"
	case *statefulpb.MetricDatum_MetricTagsetDefine:
		return "tagset_define"
	case *statefulpb.MetricDatum_MetricSeriesBatch:
		return "series_batch"
	default:
		return "unknown"
	}
}

// payloadsBuilderV3Stateful is a serieWriter that encodes series into the
// v3-columnar metric_data of a MetricSeriesBatch (dict columns 1-9 omitted) and
// submits each flush's batch plus newly-interned dict-define datums to the
// stateful gRPC sink. It shares the column-writing machinery with
// payloadsBuilderV3 but interns through a stream-scoped *StreamDictionary
// (emitting Metric*Define datums on a miss) instead of a per-payload dictionary,
// and submits over gRPC instead of accumulating a BytesPayload for HTTP.
//
// The dictionary and sink are stream-scoped (owned by the lane, passed in); the
// delta encoders, compressor, and defines slice are payload-scoped (reset per
// flush).
type payloadsBuilderV3Stateful struct {
	// outerCompression compresses the final MetricDatumSequence for the wire.
	outerCompression compression.Component
	// innerCompression is always identity: the bytes inside
	// MetricSeriesBatch.metric_data must be uncompressed MetricData proto
	// (intake parses them directly); wire compression is the outer layer.
	innerCompression compression.Component
	compressor       stream.ColumnCompressor
	txn              *stream.ColumnTransaction

	// Stream-scoped, owned by the lane. Survive across flushes.
	dict *StreamDictionary
	sink statefulgrpc.PayloadSink

	// Per-flush accumulator: newly-introduced dict-define datums.
	defines []*statefulpb.MetricDatum

	// Delta encoders, reset on each flush.
	deltaNameRef           deltaEncoder
	deltaTagsRef           deltaEncoder
	deltaResourcesRef      deltaEncoder
	deltaTimestamp         deltaEncoder
	deltaSourceTypeNameRef deltaEncoder
	deltaOriginRef         deltaEncoder

	pointsThisPayload   int
	maxPointsPerPayload int

	columnHeaderSizeBound int

	pipelineConfig  PipelineConfig
	pipelineContext *PipelineContext

	resourcesBuf []pkgmetrics.Resource
	scratchBuf   []byte

	stats v3stats
}

// newPayloadsBuilderV3StatefulWithConfig constructs a per-flush stateful builder
// bound to the lane's stream-scoped dictionary and sink, using the same payload
// sizing config as the HTTP v3 builder.
func newPayloadsBuilderV3StatefulWithConfig(
	cfg config.Component,
	comp compression.Component,
	pipelineConfig PipelineConfig,
	pipelineContext *PipelineContext,
	dict *StreamDictionary,
	sink statefulgrpc.PayloadSink,
) (*payloadsBuilderV3Stateful, error) {
	maxCompressedSize := cfg.GetInt("serializer_max_series_payload_size")
	maxUncompressedSize := cfg.GetInt("serializer_max_series_uncompressed_payload_size")
	maxPointsPerPayload := cfg.GetInt("serializer_max_series_points_per_payload")

	return newPayloadsBuilderV3Stateful(
		maxCompressedSize,
		maxUncompressedSize,
		maxPointsPerPayload,
		comp,
		pipelineConfig,
		pipelineContext,
		dict,
		sink,
	)
}

func newPayloadsBuilderV3Stateful(
	maxCompressedSize int,
	maxUncompressedSize int,
	maxPointsPerPayload int,
	comp compression.Component,
	pipelineConfig PipelineConfig,
	pipelineContext *PipelineContext,
	dict *StreamDictionary,
	sink statefulgrpc.PayloadSink,
) (*payloadsBuilderV3Stateful, error) {
	// Inner column streams are uncompressed (see innerCompression); `comp`
	// compresses the outer MetricDatumSequence for the wire.
	innerComp := selector.NewCompressor(pkgcompression.NoneKind, 0)

	columnHeaderSize := protobufFieldHeaderLen(numberOfColumns-1, maxUncompressedSize)
	columnHeaderSizeBound := innerComp.CompressBound(columnHeaderSize)
	reservedCompressedSize := columnHeaderSizeBound * numberOfColumns
	reservedUncompressedSize := columnHeaderSize * numberOfColumns
	maxCompressedSize -= reservedCompressedSize
	maxUncompressedSize -= reservedUncompressedSize
	if maxCompressedSize < 0 {
		return nil, fmt.Errorf("maxCompressedSize is too small, must be larger than %d bytes", reservedCompressedSize)
	}
	if maxUncompressedSize < 0 {
		return nil, fmt.Errorf("maxUncompressedSize is too small, must be larger than %d bytes", reservedUncompressedSize)
	}

	compressor := stream.NewColumnCompressor(
		innerComp,
		numberOfColumns,
		maxCompressedSize,
		maxUncompressedSize,
	)
	txn := compressor.NewTransaction()

	return &payloadsBuilderV3Stateful{
		outerCompression:      comp,
		innerCompression:      innerComp,
		compressor:            compressor,
		txn:                   txn,
		dict:                  dict,
		sink:                  sink,
		maxPointsPerPayload:   maxPointsPerPayload,
		columnHeaderSizeBound: columnHeaderSizeBound,
		pipelineConfig:        pipelineConfig,
		pipelineContext:       pipelineContext,
		scratchBuf:            make([]byte, columnHeaderSize),
	}, nil
}

// startPayload is a no-op for stateful — no per-payload dict to reset.
// Delta encoders are reset in finishPayload (after the previous flush
// completed) or are zero-valued on first use.
func (pb *payloadsBuilderV3Stateful) startPayload() error {
	return nil
}

// writeSerie encodes one series into the columnar batch buffer, interning
// any new dict entries into the stream dictionary as a side effect.
func (pb *payloadsBuilderV3Stateful) writeSerie(serie *pkgmetrics.Serie) error {
	if !pb.pipelineConfig.Filter.Filter(serie) {
		return nil
	}
	if ok, err := pb.checkPointsLimit(len(serie.Points)); !ok {
		return err
	}

	serie.PopulateDeviceField()
	serie.PopulateResources()

	for {
		pb.writeSerieToTxn(serie)
		err := pb.finishTxn(len(serie.Points))
		if err == errRetry {
			continue
		}
		return err
	}
}

// finishPayload finalizes the current columnar batch, assembles metric_data,
// wraps it plus the define datums into a MetricDatumSequence, marshals,
// compresses, and submits the Payload to the sink, then resets per-flush state.
func (pb *payloadsBuilderV3Stateful) finishPayload() error {
	if pb.pointsThisPayload > 0 {
		err := pb.compressor.Close()
		if err != nil {
			return err
		}

		// Size the metric_data: each present column is a protobuf field header
		// + its uncompressed bytes (dict columns 1-9 are empty, skipped).
		// metric_data is raw MetricData proto, not wrapped — intake parses it
		// directly.
		metricDataSize := 0
		for i := 0; i < numberOfColumns; i++ {
			uncompressedLen := pb.compressor.UncompressedLen(i)
			compressedBytes := pb.compressor.CompressedBytes(i)
			if uncompressedLen == 0 {
				continue
			}
			metricDataSize += protobufFieldHeaderLen(i, uncompressedLen) + len(compressedBytes)

			tlmColumnSize.Add(float64(len(compressedBytes)), columnNames[i], "compressed")
			tlmColumnSize.Add(float64(uncompressedLen), columnNames[i], "uncompressed")
		}

		// Assemble metric_data. Since innerCompression is identity, the
		// compressed bytes returned by the ColumnCompressor are the raw
		// uncompressed proto field bytes, and appendProtobufFieldHeader
		// emits raw field headers — so the concatenation is valid
		// uncompressed MetricData proto.
		metricData := make([]byte, 0, metricDataSize)
		for i := 0; i < numberOfColumns; i++ {
			uncompressedLen := pb.compressor.UncompressedLen(i)
			compressedBytes := pb.compressor.CompressedBytes(i)
			if uncompressedLen == 0 {
				continue
			}
			metricData, err = pb.appendProtobufFieldHeader(metricData, i, uncompressedLen)
			if err != nil {
				return err
			}
			metricData = append(metricData, compressedBytes...)
		}

		// Submit to the stateful sender.
		if err := pb.submit(metricData); err != nil {
			return err
		}
	}

	pb.updateValuesStats()
	pb.resetForNextPayload()
	return nil
}

// submit packages the metric_data bytes + newly-defined datums into a
// MetricDatumSequence, marshals + compresses, and hands to the sender.
//
// Sequence ordering follows the wire invariant: all defines first, then
// the MetricSeriesBatch (which references entries defined in the same
// sequence or earlier on the stream).
func (pb *payloadsBuilderV3Stateful) submit(metricData []byte) error {
	// Defines + the series-batch datum.
	datums := make([]*statefulpb.MetricDatum, 0, len(pb.defines)+1)
	datums = append(datums, pb.defines...)
	datums = append(datums, &statefulpb.MetricDatum{
		Data: &statefulpb.MetricDatum_MetricSeriesBatch{
			MetricSeriesBatch: &statefulpb.MetricSeriesBatch{MetricData: metricData},
		},
	})
	// Per-datum-kind telemetry. Counted before serialization since
	// MarshalVT consumes the slice.
	for _, d := range datums {
		kind := datumKindName(d)
		tlmStatefulDatumCount.Inc(kind)
		tlmStatefulDatumBytes.Add(float64(proto.Size(d)), kind)
	}
	seq := &statefulpb.MetricDatumSequence{Data: datums}
	serialized, err := protoMarshalMetricDatumSequence(seq)
	if err != nil {
		return fmt.Errorf("stateful v3: marshal: %w", err)
	}

	// Outer-compress the serialized sequence for the wire (the metric_data
	// inside stays uncompressed proto).
	encoded, err := pb.outerCompression.Compress(serialized)
	if err != nil {
		return fmt.Errorf("stateful v3: compress: %w", err)
	}

	encoding := pkgcompression.NoneKind
	if pb.outerCompression != nil {
		encoding = pb.outerCompression.ContentEncoding()
	}

	// stateChanges = the define datums (NOT the MetricSeriesBatch). Used by
	// the inflight tracker to apply to the snapshot on ack.
	stateChanges := make([]*statefulpb.MetricDatum, len(pb.defines))
	copy(stateChanges, pb.defines)

	payload := &statefulgrpc.Payload{
		Encoded:             encoded,
		Encoding:            encoding,
		UnencodedSize:       len(serialized),
		PreCompressionBytes: len(serialized),
		PointCount:          pb.pointsThisPayload,
		StateChanges:        stateChanges,
	}
	return pb.sink.Submit(payload)
}

func (pb *payloadsBuilderV3Stateful) appendProtobufFieldHeader(dst []byte, id int, ln int) ([]byte, error) {
	n := binary.PutUvarint(pb.scratchBuf[0:], protobufFieldID(id, pbTypeBytes))
	n += binary.PutUvarint(pb.scratchBuf[n:], uint64(ln))
	// innerCompression is identity, so this is a pass-through (the field header
	// must be raw bytes in the uncompressed metric_data).
	header, err := pb.innerCompression.Compress(pb.scratchBuf[:n])
	if err != nil {
		return nil, err
	}
	return append(dst, header...), nil
}

// resetForNextPayload clears the per-flush state. Stream-scoped state
// (dict) is preserved.
func (pb *payloadsBuilderV3Stateful) resetForNextPayload() {
	pb.pointsThisPayload = 0
	pb.defines = pb.defines[:0]
	pb.deltaNameRef.reset()
	pb.deltaTagsRef.reset()
	pb.deltaResourcesRef.reset()
	pb.deltaTimestamp.reset()
	pb.deltaSourceTypeNameRef.reset()
	pb.deltaOriginRef.reset()
	pb.compressor.Reset()
	pb.stats = v3stats{}
}

// --- per-series writers -------------

func (pb *payloadsBuilderV3Stateful) renderResources(serie *pkgmetrics.Serie) {
	pb.resourcesBuf = pb.resourcesBuf[0:0]
	if serie.Host != "" {
		pb.resourcesBuf = append(pb.resourcesBuf, pkgmetrics.Resource{
			Type: resourceTypeHost,
			Name: serie.Host,
		})
	}
	if serie.Device != "" {
		pb.resourcesBuf = append(pb.resourcesBuf, pkgmetrics.Resource{
			Type: "device",
			Name: serie.Device,
		})
	}
	pb.resourcesBuf = append(pb.resourcesBuf, serie.Resources...)
}

func (pb *payloadsBuilderV3Stateful) checkPointsLimit(numPoints int) (bool, error) {
	if numPoints == 0 {
		return false, nil
	}
	if numPoints > pb.maxPointsPerPayload {
		tlmItemTooBig.Inc()
		return false, nil
	}
	if numPoints+pb.pointsThisPayload > pb.maxPointsPerPayload {
		tlmSplitReason.Inc("max_points")
		if err := pb.finishPayload(); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (pb *payloadsBuilderV3Stateful) finishTxn(numPoints int) error {
	err := pb.compressor.AddItem(pb.txn)
	switch {
	case errors.Is(err, stream.ErrPayloadFull):
		tlmSplitReason.Inc("payload_full")
		if ferr := pb.finishPayload(); ferr != nil {
			return ferr
		}
		return errRetry
	case errors.Is(err, stream.ErrItemTooBig):
		tlmItemTooBig.Inc()
		tlmSplitReason.Inc("item_too_big")
		return pb.finishPayload()
	case err == nil:
		pb.pointsThisPayload += numPoints
		return nil
	default:
		return err
	}
}

// writeMetricCommon writes the per-series "header" columns (Type, NameRef,
// TagsRef, ResourcesRef, Interval, NumPoints, SourceTypeNameRef, OriginRef)
// into the columnar txn, interning any new dict entries.
func (pb *payloadsBuilderV3Stateful) writeMetricCommon(
	name string,
	tags tagset.CompositeTags,
	interval int64,
	sourceTypeName string,
	source pkgmetrics.MetricSource,
	numPoints int,
) {
	nameID := pb.dict.InternName(name, &pb.defines)
	tagsID := pb.dict.InternTags(tags, &pb.defines)
	resourcesID := pb.dict.InternResources(pb.resourcesBuf, &pb.defines)
	stnID := pb.dict.InternSourceTypeName(sourceTypeName, &pb.defines)
	originID := pb.dict.InternOriginInfo(originInfo{
		product:  metricSourceToOriginProduct(source),
		category: metricSourceToOriginCategory(source),
		service:  metricSourceToOriginService(source),
	}, &pb.defines)

	pb.txn.Sint64(columnNameRef, pb.deltaNameRef.encode(int64(nameID)))
	pb.txn.Sint64(columnTagsRef, pb.deltaTagsRef.encode(int64(tagsID)))
	pb.txn.Int64(columnInterval, interval)
	pb.txn.Sint64(columnResourcesRef, pb.deltaResourcesRef.encode(int64(resourcesID)))
	pb.txn.Sint64(columnSourceTypeNameRef, pb.deltaSourceTypeNameRef.encode(int64(stnID)))
	pb.txn.Sint64(columnOriginRef, pb.deltaOriginRef.encode(int64(originID)))
	pb.txn.Int64(columnNumPoints, int64(numPoints))
}

func (pb *payloadsBuilderV3Stateful) writePointCommon(timestamp int64) {
	pb.txn.Sint64(columnTimestamp, pb.deltaTimestamp.encode(timestamp))
}

func (pb *payloadsBuilderV3Stateful) writeSerieToTxn(serie *pkgmetrics.Serie) {
	pb.txn.Reset()

	pb.renderResources(serie)

	pb.writeMetricCommon(
		serie.Name,
		serie.Tags,
		serie.Interval,
		serie.SourceTypeName,
		serie.Source,
		len(serie.Points),
	)

	pointKind := pointKindZero
	for _, pnt := range serie.Points {
		pointKind = pointKind.unionOf(pnt.Value)
	}
	valueType := pointKind.toValueType()

	typeValue := valueType | metricType(serie.MType)
	if serie.NoIndex {
		typeValue |= flagNoIndex
	}
	pb.txn.Int64(columnType, typeValue)

	for _, pnt := range serie.Points {
		pb.writePointCommon(int64(pnt.Ts))
		switch valueType {
		case valueZero:
			pb.stats.valuesZero++
		case valueSint64:
			pb.stats.valuesSint64++
			pb.txn.Sint64(columnValueSint64, int64(pnt.Value))
		case valueFloat32:
			pb.stats.valuesFloat32++
			pb.txn.Float32(columnValueFloat32, float32(pnt.Value))
		case valueFloat64:
			pb.stats.valuesFloat64++
			pb.txn.Float64(columnValueFloat64, pnt.Value)
		}
	}
}

func (pb *payloadsBuilderV3Stateful) updateValuesStats() {
	tlmValuesCount.Add(float64(pb.stats.valuesZero), "zero")
	tlmValuesCount.Add(float64(pb.stats.valuesSint64), "sint64")
	tlmValuesCount.Add(float64(pb.stats.valuesFloat32), "float32")
	tlmValuesCount.Add(float64(pb.stats.valuesFloat64), "float64")
}

// protoMarshalMetricDatumSequence marshals the sequence; a function seam for
// tests.
func protoMarshalMetricDatumSequence(seq *statefulpb.MetricDatumSequence) ([]byte, error) {
	return seq.MarshalVT()
}
