// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"bytes"
	"errors"
	"fmt"

	jsoniter "github.com/json-iterator/go"
	"github.com/richardartoul/molecule"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// IterableSeries is a serializer for metrics.IterableSeries
type IterableSeries struct {
	source metrics.SerieSource
}

// CreateIterableSeries creates a new instance of *IterableSeries
func CreateIterableSeries(source metrics.SerieSource) *IterableSeries {
	return &IterableSeries{
		source: source,
	}
}

// MoveNext moves to the next item.
// This function skips the series when `NoIndex` is set at true as `NoIndex` is only supported by `MarshalSplitCompress`.
func (series *IterableSeries) MoveNext() bool {
	res := series.source.MoveNext()
	for res {
		serie := series.source.Current()
		if serie == nil || !serie.NoIndex {
			break
		}
		// Skip noIndex metric
		res = series.source.MoveNext()
	}
	return res
}

// WriteHeader writes the payload header for this type
func (series *IterableSeries) WriteHeader(stream *jsoniter.Stream) error {
	return writeHeader(stream)
}

func writeHeader(stream *jsoniter.Stream) error {
	stream.WriteObjectStart()
	stream.WriteObjectField("series")
	stream.WriteArrayStart()
	return stream.Flush()
}

// WriteFooter writes the payload footer for this type
func (series *IterableSeries) WriteFooter(stream *jsoniter.Stream) error {
	return writeFooter(stream)
}

func writeFooter(stream *jsoniter.Stream) error {
	stream.WriteArrayEnd()
	stream.WriteObjectEnd()
	return stream.Flush()
}

// WriteCurrentItem writes the json representation of an item
func (series *IterableSeries) WriteCurrentItem(stream *jsoniter.Stream) error {
	current := series.source.Current()
	if current == nil {
		return errors.New("nil serie")
	}
	return writeItem(stream, current)
}

func writeItem(stream *jsoniter.Stream, serie *metrics.Serie) error {
	serie.PopulateDeviceField()
	serie.PopulateResources()
	encodeSerie(serie, stream)
	return stream.Flush()
}

// DescribeCurrentItem returns a text description for logs
func (series *IterableSeries) DescribeCurrentItem() string {
	current := series.source.Current()
	if current == nil {
		return "nil serie"
	}
	return describeItem(current)
}

// GetCurrentItemPointCount gets the number of points in the current serie
func (series *IterableSeries) GetCurrentItemPointCount() int {
	return len(series.source.Current().Points)
}

func describeItem(serie *metrics.Serie) string {
	return fmt.Sprintf("name %q, %d points", serie.Name, len(serie.Points))
}

// Filterable defines the minimal interface needed for pipeline filtering. Both
// Serie and SketchSeries implement this interface.
type Filterable interface {
	GetName() string
}

// Pipeline represents a data processing pipeline that filters metrics and marks
// them for a specific destination
type Pipeline struct {
	FilterFunc  func(metric Filterable) bool
	Destination transaction.Destination
}

// MarshalSplitCompressPipelines uses the stream compressor to marshal and
// compress series payloads, allowing multiple variants to be generated in a
// single pass over the input data. If a compressed payload is larger than the
// max, a new payload will be generated. This method returns a slice of
// compressed protobuf marshaled MetricPayload objects.
func (series *IterableSeries) MarshalSplitCompressPipelines(config config.Component, strategy compression.Component, pipelines []Pipeline) (transaction.BytesPayloads, error) {
	pbs := make([]*PayloadsBuilder, len(pipelines))
	for i := range pbs {
		bufferContext := marshaler.NewBufferContext()
		pb, err := series.NewPayloadsBuilder(bufferContext, config, strategy)
		if err != nil {
			return nil, err
		}
		pbs[i] = &pb

		err = pbs[i].startPayload()
		if err != nil {
			return nil, err
		}
	}

	// Use series.source.MoveNext() instead of series.MoveNext() because this function supports
	// the serie.NoIndex field.
	for series.source.MoveNext() {
		for i, pipeline := range pipelines {
			if pipeline.FilterFunc(series.source.Current()) {
				err := pbs[i].writeSerie(series.source.Current())
				if err != nil {
					return nil, err
				}
			}
		}
	}

	// if the last payload has any data, flush it
	for i := range pbs {
		err := pbs[i].finishPayload()
		if err != nil {
			return nil, err
		}
	}

	// assign destinations to payloads per strategy
	for i, pipeline := range pipelines {
		for _, payload := range pbs[i].payloads {
			payload.Destination = pipeline.Destination
		}
	}

	totalPayloads := 0
	for _, pb := range pbs {
		totalPayloads += len(pb.payloads)
	}
	payloads := make([]*transaction.BytesPayload, 0, totalPayloads)
	for _, pb := range pbs {
		payloads = append(payloads, pb.payloads...)
	}

	return payloads, nil
}

// NewPayloadsBuilder initializes a new PayloadsBuilder to be used for serializing series into a set of output payloads.
func (series *IterableSeries) NewPayloadsBuilder(bufferContext *marshaler.BufferContext, config config.Component, strategy compression.Component) (PayloadsBuilder, error) {
	buf := bufferContext.PrecompressionBuf
	ps := molecule.NewProtoStream(buf)

	return PayloadsBuilder{
		bufferContext: bufferContext,
		config:        config,
		strategy:      strategy,

		compressor: nil,
		buf:        buf,
		ps:         ps,
		payloads:   []*transaction.BytesPayload{},

		pointsThisPayload: 0,
		seriesThisPayload: 0,

		maxPayloadSize:      config.GetInt("serializer_max_series_payload_size"),
		maxUncompressedSize: config.GetInt("serializer_max_series_uncompressed_payload_size"),
		maxPointsPerPayload: config.GetInt("serializer_max_series_points_per_payload"),
	}, nil
}

// PayloadsBuilder represents an in-progress serialization of a series into potentially multiple payloads.
type PayloadsBuilder struct {
	bufferContext *marshaler.BufferContext
	config        config.Component
	strategy      compression.Component

	compressor *stream.Compressor
	buf        *bytes.Buffer
	ps         *molecule.ProtoStream
	payloads   []*transaction.BytesPayload

	pointsThisPayload int
	seriesThisPayload int

	maxPayloadSize      int
	maxUncompressedSize int
	maxPointsPerPayload int
}

// Prepare to write the next payload
func (pb *PayloadsBuilder) startPayload() error {
	pb.pointsThisPayload = 0
	pb.seriesThisPayload = 0
	pb.bufferContext.CompressorInput.Reset()
	pb.bufferContext.CompressorOutput.Reset()

	compressor, err := stream.NewCompressor(
		pb.bufferContext.CompressorInput, pb.bufferContext.CompressorOutput,
		pb.maxPayloadSize, pb.maxUncompressedSize,
		[]byte{}, []byte{}, []byte{}, pb.strategy)
	if err != nil {
		return err
	}
	pb.compressor = compressor

	return nil
}

func (pb *PayloadsBuilder) writeSerie(serie *metrics.Serie) error {
	// constants for the protobuf data we will be writing, taken from MetricPayload in
	// https://github.com/DataDog/agent-payload/blob/master/proto/metrics/agent_payload.proto
	const payloadSeries = 1
	const seriesResources = 1
	const seriesMetric = 2
	const seriesTags = 3
	const seriesPoints = 4
	const seriesType = 5
	const seriesSourceTypeName = 7
	const seriesInterval = 8
	const serieMetadata = 9
	const resourceType = 1
	const resourceName = 2
	const pointValue = 1
	const pointTimestamp = 2
	const serieMetadataOrigin = 1
	//         |------| 'Metadata' message
	//                 |-----| 'origin' field index
	const serieMetadataOriginMetricType = 3
	//         |------| 'Metadata' message
	//                 |----| 'origin' message
	//                       |--------| 'metric_type' field index
	const metryTypeNotIndexed = 9
	//    |-----------------| 'metric_type_agent_hidden' field index

	const serieMetadataOriginOriginProduct = 4
	//                 |----|  'Origin' message
	//                       |-----------| 'origin_product' field index
	const serieMetadataOriginOriginCategory = 5
	//                 |----|  'Origin' message
	//                       |-----------| 'origin_category' field index
	const serieMetadataOriginOriginService = 6
	//                 |----|  'Origin' message
	//                       |-----------| 'origin_service' field index

	addToPayload := func() error {
		err := pb.compressor.AddItem(pb.buf.Bytes())
		if err != nil {
			return err
		}
		pb.pointsThisPayload += len(serie.Points)
		pb.seriesThisPayload++
		return nil
	}

	serie.PopulateDeviceField()
	serie.PopulateResources()

	pb.buf.Reset()
	err := pb.ps.Embedded(payloadSeries, func(ps *molecule.ProtoStream) error {
		var err error

		err = ps.Embedded(seriesResources, func(ps *molecule.ProtoStream) error {
			err = ps.String(resourceType, "host")
			if err != nil {
				return err
			}

			return ps.String(resourceName, serie.Host)
		})
		if err != nil {
			return err
		}

		if serie.Device != "" {
			err = ps.Embedded(seriesResources, func(ps *molecule.ProtoStream) error {
				err = ps.String(resourceType, "device")
				if err != nil {
					return err
				}

				return ps.String(resourceName, serie.Device)
			})
			if err != nil {
				return err
			}
		}

		if len(serie.Resources) > 0 {
			for _, r := range serie.Resources {
				err = ps.Embedded(seriesResources, func(ps *molecule.ProtoStream) error {
					err = ps.String(resourceType, r.Type)
					if err != nil {
						return err
					}

					return ps.String(resourceName, r.Name)
				})
				if err != nil {
					return err
				}
			}
		}

		err = ps.String(seriesMetric, serie.Name)
		if err != nil {
			return err
		}

		err = serie.Tags.ForEachErr(func(tag string) error {
			return ps.String(seriesTags, tag)
		})
		if err != nil {
			return err
		}

		err = ps.Int32(seriesType, serie.MType.SeriesAPIV2Enum())
		if err != nil {
			return err
		}

		err = ps.String(seriesSourceTypeName, serie.SourceTypeName)
		if err != nil {
			return err
		}

		err = ps.Int64(seriesInterval, serie.Interval)
		if err != nil {
			return err
		}

		// (Unit is omitted)

		for _, p := range serie.Points {
			err = ps.Embedded(seriesPoints, func(ps *molecule.ProtoStream) error {
				err = ps.Int64(pointTimestamp, int64(p.Ts))
				if err != nil {
					return err
				}

				err = ps.Double(pointValue, p.Value)
				if err != nil {
					return err
				}

				return nil
			})
			if err != nil {
				return err
			}
		}

		return ps.Embedded(serieMetadata, func(ps *molecule.ProtoStream) error {
			return ps.Embedded(serieMetadataOrigin, func(ps *molecule.ProtoStream) error {
				if serie.NoIndex {
					err = ps.Int32(serieMetadataOriginMetricType, metryTypeNotIndexed)
					if err != nil {
						return err
					}
				}
				err = ps.Int32(serieMetadataOriginOriginProduct, metricSourceToOriginProduct(serie.Source))
				if err != nil {
					return err
				}
				err = ps.Int32(serieMetadataOriginOriginCategory, metricSourceToOriginCategory(serie.Source))
				if err != nil {
					return err
				}
				return ps.Int32(serieMetadataOriginOriginService, metricSourceToOriginService(serie.Source))
			})
		})
	})
	if err != nil {
		return err
	}

	if len(serie.Points) > pb.maxPointsPerPayload {
		// this series is just too big to fit in a payload (even alone)
		err = stream.ErrItemTooBig
	} else if pb.pointsThisPayload+len(serie.Points) > pb.maxPointsPerPayload {
		// this series won't fit in this payload, but will fit in the next
		err = stream.ErrPayloadFull
	} else {
		// Compress the protobuf metadata and the marshaled series
		err = addToPayload()
	}

	switch err {
	case stream.ErrPayloadFull:
		expvarsPayloadFull.Add(1)
		tlmPayloadFull.Inc()

		err = pb.finishPayload()
		if err != nil {
			return err
		}

		err = pb.startPayload()
		if err != nil {
			return err
		}

		// Add it to the new compression buffer
		err = addToPayload()
		if err == stream.ErrItemTooBig {
			// Since it was too big to fit into a empty payload, there is
			// nothing left to do but track the failure and drop the item.
			// Returning nil here lets us continue adding any other items to the
			// payload.
			expvarsItemTooBig.Add(1)
			tlmItemTooBig.Inc()
			return nil
		}
		if err != nil {
			// Unexpected error bail out
			expvarsUnexpectedItemDrops.Add(1)
			tlmUnexpectedItemDrops.Inc()
			return err
		}
	case stream.ErrItemTooBig:
		// Item was too big, drop it
		expvarsItemTooBig.Add(1)
		tlmItemTooBig.Add(1)
	case nil:
		// Item successfully written to payload
		return nil
	default:
		// Unexpected error bail out
		expvarsUnexpectedItemDrops.Add(1)
		tlmUnexpectedItemDrops.Inc()
		return err
	}

	return nil
}

func (pb *PayloadsBuilder) finishPayload() error {
	var payload []byte
	// Since the compression buffer is full - flush it and rotate
	payload, err := pb.compressor.Close()
	if err != nil {
		return err
	}

	if pb.seriesThisPayload > 0 {
		pb.payloads = append(pb.payloads, transaction.NewBytesPayload(payload, pb.pointsThisPayload))
	}

	return nil
}

func encodeSerie(serie *metrics.Serie, stream *jsoniter.Stream) {
	stream.WriteObjectStart()

	stream.WriteObjectField("metric")
	stream.WriteString(serie.Name)
	stream.WriteMore()

	stream.WriteObjectField("points")
	encodePoints(serie.Points, stream)
	stream.WriteMore()

	stream.WriteObjectField("tags")
	stream.WriteArrayStart()
	firstTag := true
	serie.Tags.ForEach(func(s string) {
		if !firstTag {
			stream.WriteMore()
		}
		stream.WriteString(s)
		firstTag = false
	})
	stream.WriteArrayEnd()
	stream.WriteMore()

	stream.WriteObjectField("host")
	stream.WriteString(serie.Host)
	stream.WriteMore()

	if serie.Device != "" {
		stream.WriteObjectField("device")
		stream.WriteString(serie.Device)
		stream.WriteMore()
	}

	stream.WriteObjectField("type")
	stream.WriteString(serie.MType.String())
	stream.WriteMore()

	stream.WriteObjectField("interval")
	stream.WriteInt64(serie.Interval)

	if serie.SourceTypeName != "" {
		stream.WriteMore()
		stream.WriteObjectField("source_type_name")
		stream.WriteString(serie.SourceTypeName)
	}

	stream.WriteObjectEnd()
}

func encodePoints(points []metrics.Point, stream *jsoniter.Stream) {
	var needComa bool

	stream.WriteArrayStart()
	for _, p := range points {
		if needComa {
			stream.WriteMore()
		} else {
			needComa = true
		}
		stream.WriteArrayStart()
		stream.WriteInt64(int64(p.Ts))
		stream.WriteMore()
		stream.WriteFloat64(p.Value)
		stream.WriteArrayEnd()
	}
	stream.WriteArrayEnd()
}
