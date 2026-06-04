// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	statefulgrpc "github.com/DataDog/datadog-agent/pkg/serializer/metrics/grpc"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	pkgcompression "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Per-datum-kind telemetry — count and bytes emitted by the encoder.
// Counters declared in pkg/serializer/metrics/grpc/state_telemetry.go would
// fit better, but importing that package from the encoder pulls in gRPC
// deps unnecessarily. Declaring here keeps the encoder's only telemetry
// import in this file. The naming convention matches contract.md D10.
var (
	tlmStatefulDatumCount = telemetry.NewCounter(
		"serializer", "v3_stateful_datum_count",
		[]string{"kind"},
		"Count of metric datum emitted by the stateful v3 encoder, by kind",
	)
	tlmStatefulDatumBytes = telemetry.NewCounter(
		"serializer", "v3_stateful_datum_bytes",
		[]string{"kind"},
		"Bytes of metric datum emitted by the stateful v3 encoder, by kind (proto.Size)",
	)
)

// datumKindName returns the contract.md D10 telemetry-kind label for a
// MetricDatum. Returns "unknown" if the datum's data oneof is set to a
// type not enumerated here (defensive — should not happen).
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

// payloadsBuilderV3Stateful is the third serieWriter implementation alongside
// payloadsBuilderV3 (HTTP v3 columnar) and PayloadsBuilder (HTTP v2 row-
// oriented). It produces the metric_data bytes of a MetricSeriesBatch
// (apiv3 columnar layout with dict columns 1-9 omitted) and submits each
// flush's batch + new dict-define datums to the stateful gRPC sender.
//
// Most of the column-writing machinery is identical to payloadsBuilderV3
// (writeMetricCommon, writePointCommon, the per-payload delta encoders, the
// pointKind/value-type adaptive selection). The differences are:
//
//   - Dictionary lookups go through *StreamDictionary (stream-scoped,
//     persists across flushes) instead of the per-payload dictionaryBuilder.
//   - Dict columns 1-9 are never written; instead, newly-interned entries
//     produce MetricXxxDefine datums collected in `defines`.
//   - finishPayload assembles the metric_data bytes (same envelope format
//     as v3 but missing the dict columns), wraps it in a MetricSeriesBatch
//     datum, prepends the defines as a MetricDatumSequence, marshals,
//     optionally compresses, and submits to the sender. It does NOT add a
//     transaction.BytesPayload to pipelineContext (the stateful path
//     bypasses the HTTP forwarder).
//
// Stream-scope vs payload-scope state:
//
//   - dict (StreamDictionary):                  stream-scoped (persists across flushes)
//   - delta encoders (deltaNameRef, etc):       payload-scoped (reset per flush)
//   - compressor (ColumnCompressor):            payload-scoped (recreated per flush)
//   - defines slice:                            payload-scoped (per-flush accumulator)
//
// Stream-scope state lives in the lane (here passed in as `dict` + `sender`).
// Payload-scope state lives in this struct, recreated per call to
// newPayloadsBuilderV3StatefulWithConfig.

type payloadsBuilderV3Stateful struct {
	// outerCompression compresses the final MetricDatumSequence bytes
	// before they ride the gRPC stream. Configured by the agent (zstd/
	// gzip/none).
	outerCompression compression.Component
	// innerCompression is always identity ("none") — passed to the
	// ColumnCompressor and to appendProtobufFieldHeader. The bytes inside
	// MetricSeriesBatch.metric_data MUST be uncompressed MetricData proto
	// per contract.md D11.4 (intake parses them directly as MetricData
	// without a decompression step). Outer compression at the
	// MetricDatumSequence layer reclaims the column compression win.
	innerCompression compression.Component
	compressor       stream.ColumnCompressor
	txn              *stream.ColumnTransaction

	// Stream-scoped, owned by the lane. Survives across flushes.
	dict   *StreamDictionary
	sender *statefulgrpc.Sender

	// Per-flush accumulator: newly-introduced dict-define datums.
	defines []*statefulpb.MetricDatum

	// Delta encoders — batch-scoped per contract.md D6 (reset on each flush).
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

// newPayloadsBuilderV3StatefulWithConfig constructs a stateful v3 builder
// using the same sizing logic as the HTTP v3 builder, but bound to the
// supplied stream-scoped dictionary and gRPC sender. The dict and sender
// are owned by the serializer (one per lane); the builder is created
// fresh per flush.
func newPayloadsBuilderV3StatefulWithConfig(
	cfg config.Component,
	comp compression.Component,
	pipelineConfig PipelineConfig,
	pipelineContext *PipelineContext,
	dict *StreamDictionary,
	sender *statefulgrpc.Sender,
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
		sender,
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
	sender *statefulgrpc.Sender,
) (*payloadsBuilderV3Stateful, error) {
	// The inner column streams MUST be uncompressed — see field doc on
	// innerCompression and contract.md D11.4. Outer compression at the
	// MetricDatumSequence layer (using `comp`) handles wire compression.
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
		sender:                sender,
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

// finishPayload finalizes the current columnar batch (if any), assembles
// the metric_data envelope, wraps it with newly-introduced define datums
// into a MetricDatumSequence, marshals, optionally compresses, and
// submits the resulting Payload to the gRPC sender. Then resets per-flush
// state.
//
// The pipelineContext is intentionally NOT mutated — stateful pipelines do
// not produce BytesPayloads for the HTTP forwarder.
func (pb *payloadsBuilderV3Stateful) finishPayload() error {
	if pb.pointsThisPayload > 0 {
		err := pb.compressor.Close()
		if err != nil {
			return err
		}

		// Compute the total size of the metric_data bytes. Each present
		// column contributes: protobuf field header + uncompressed column
		// bytes. Dict columns 1-9 are always empty and skipped.
		//
		// NOTE: there is NO outer Payload wrapper. metric_data is a raw
		// MetricData proto message — its fields are the per-series and
		// per-point column fields (10-19, 23, 24) directly. Per
		// contract.md D11.4, intake parses metric_data as MetricData
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

	// Compress the DatumSequence bytes for the wire. This is the OUTER
	// compression layer. metric_data inside is uncompressed proto (per
	// contract.md D11.4); outer zstd handles wire-size reduction.
	encoded, err := pb.outerCompression.Compress(serialized)
	if err != nil {
		return fmt.Errorf("stateful v3: compress: %w", err)
	}

	// Diagnostic: dump the first emitted metric_data + compressed
	// DatumSequence to /tmp so the architect can round-trip them through
	// tools/v3-decode to verify wire correctness. Gated by sync.Once so
	// it happens exactly once per process. PoC-only.
	debugDumpFirstSubmit(metricData, serialized, encoded, len(pb.defines))

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
	return pb.sender.Submit(payload)
}

// debugDumpFirstSubmit writes the wire-format bytes of the first stateful
// metric batch to /tmp/ for offline inspection. Used to verify the inner
// MetricData bytes round-trip through dd-source/apiv3 cleanly. Fires
// exactly once per process.
var debugDumpOnce sync.Once

func debugDumpFirstSubmit(metricData, datumSeq, encoded []byte, numDefines int) {
	debugDumpOnce.Do(func() {
		paths := map[string][]byte{
			"/tmp/stateful-metrics-debug-metric_data.bin":    metricData,
			"/tmp/stateful-metrics-debug-datum_sequence.bin": datumSeq,
			"/tmp/stateful-metrics-debug-encoded.bin":        encoded,
		}
		for path, data := range paths {
			if err := os.WriteFile(path, data, 0644); err != nil {
				log.Warnf("stateful v3: failed to write debug dump %s: %v", path, err)
				return
			}
		}
		log.Infof("stateful v3: first batch dumped — metric_data=%d B (raw MetricData proto), datum_sequence=%d B (serialized), encoded=%d B (compressed wire), defines=%d. Verify via: cd tools/v3-decode && go run . < /tmp/stateful-metrics-debug-metric_data.bin (note: tools/v3-decode expects a Payload, so wrap field 3 if needed)",
			len(metricData), len(datumSeq), len(encoded), numDefines)
	})
}

func (pb *payloadsBuilderV3Stateful) appendProtobufFieldHeader(dst []byte, id int, ln int) ([]byte, error) {
	n := binary.PutUvarint(pb.scratchBuf[0:], protobufFieldID(id, pbTypeBytes))
	n += binary.PutUvarint(pb.scratchBuf[n:], uint64(ln))
	// innerCompression is identity ("none") — Compress is effectively a
	// pass-through. The metric_data bytes must be uncompressed proto
	// (contract.md D11.4); outer compression happens at the
	// MetricDatumSequence layer.
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

// --- per-series writers (mostly mirror payloadsBuilderV3) -------------

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

// protoMarshalMetricDatumSequence is a wrapper around proto.Marshal
// extracted to a function so tests can mock it. Currently a thin pass-
// through.
func protoMarshalMetricDatumSequence(seq *statefulpb.MetricDatumSequence) ([]byte, error) {
	return seq.MarshalVT()
}
