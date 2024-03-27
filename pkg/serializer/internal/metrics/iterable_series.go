// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	jsoniter "github.com/json-iterator/go"
	"github.com/richardartoul/molecule"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
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

// MarshalSplitCompress uses the stream compressor to marshal and compress series payloads.
// If a compressed payload is larger than the max, a new payload will be generated. This method returns a slice of
// compressed protobuf marshaled MetricPayload objects.
func (series *IterableSeries) MarshalSplitCompress(bufferContext *marshaler.BufferContext, config config.Component, strategy compression.Component) (transaction.BytesPayloads, error) {
	var err error
	var compressor *stream.Compressor
	buf := bufferContext.PrecompressionBuf
	ps := molecule.NewProtoStream(buf)
	payloads := transaction.BytesPayloads{}

	var pointsThisPayload int
	var seriesThisPayload int
	var serie *metrics.Serie

	// the backend accepts payloads up to specific compressed / uncompressed
	// sizes, but prefers small uncompressed payloads.  For series, there is
	// also a maximum number of points.
	maxPayloadSize := config.GetInt("serializer_max_series_payload_size")
	maxUncompressedSize := config.GetInt("serializer_max_series_uncompressed_payload_size")
	maxPointsPerPayload := config.GetInt("serializer_max_series_points_per_payload")

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
	const serieMetadataOriginOriginProductAgentType = 10
	//                 |----|  'Origin' message
	//                       |-----------| 'OriginProduct' enum
	//                                    |-------| 'Agent' enum value

	// Prepare to write the next payload
	startPayload := func() error {
		var err error

		pointsThisPayload = 0
		seriesThisPayload = 0
		bufferContext.CompressorInput.Reset()
		bufferContext.CompressorOutput.Reset()

		compressor, err = stream.NewCompressor(
			bufferContext.CompressorInput, bufferContext.CompressorOutput,
			maxPayloadSize, maxUncompressedSize,
			[]byte{}, []byte{}, []byte{}, strategy)
		if err != nil {
			return err
		}

		return nil
	}

	addToPayload := func() error {
		err = compressor.AddItem(buf.Bytes())
		if err != nil {
			return err
		}
		pointsThisPayload += len(serie.Points)
		seriesThisPayload++
		return nil
	}

	finishPayload := func() error {
		var payload []byte
		// Since the compression buffer is full - flush it and rotate
		payload, err = compressor.Close()
		if err != nil {
			return err
		}

		if seriesThisPayload > 0 {
			payloads = append(payloads, transaction.NewBytesPayload(payload, pointsThisPayload))
		}

		return nil
	}

	// start things off
	err = startPayload()
	if err != nil {
		return nil, err
	}

	// Use series.source.MoveNext() instead of series.MoveNext() because this function supports
	// the serie.NoIndex field.
	for series.source.MoveNext() {
		serie = series.source.Current()
		serie.PopulateDeviceField()
		serie.PopulateResources()

		buf.Reset()
		err = ps.Embedded(payloadSeries, func(ps *molecule.ProtoStream) error {
			var err error

			err = ps.Embedded(seriesResources, func(ps *molecule.ProtoStream) error {
				err = ps.String(resourceType, "host")
				if err != nil {
					return err
				}

				err = ps.String(resourceName, serie.Host)
				if err != nil {
					return err
				}

				return nil
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

					err = ps.String(resourceName, serie.Device)
					if err != nil {
						return err
					}

					return nil
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

						err = ps.String(resourceName, r.Name)
						if err != nil {
							return err
						}

						return nil
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

			err = ps.Embedded(serieMetadata, func(ps *molecule.ProtoStream) error {
				return ps.Embedded(serieMetadataOrigin, func(ps *molecule.ProtoStream) error {
					if serie.NoIndex {
						err = ps.Int32(serieMetadataOriginMetricType, metryTypeNotIndexed)
						if err != nil {
							return err
						}
					}
					err = ps.Int32(serieMetadataOriginOriginProduct, serieMetadataOriginOriginProductAgentType)
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
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		if len(serie.Points) > maxPointsPerPayload {
			// this series is just too big to fit in a payload (even alone)
			err = stream.ErrItemTooBig
		} else if pointsThisPayload+len(serie.Points) > maxPointsPerPayload {
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

			err = finishPayload()
			if err != nil {
				return nil, err
			}

			err = startPayload()
			if err != nil {
				return nil, err
			}

			// Add it to the new compression buffer
			err = addToPayload()
			if err == stream.ErrItemTooBig {
				// Item was too big, drop it
				expvarsItemTooBig.Add(1)
				tlmItemTooBig.Inc()
				continue
			}
			if err != nil {
				// Unexpected error bail out
				expvarsUnexpectedItemDrops.Add(1)
				tlmUnexpectedItemDrops.Inc()
				return nil, err
			}
		case stream.ErrItemTooBig:
			// Item was too big, drop it
			expvarsItemTooBig.Add(1)
			tlmItemTooBig.Add(1)
		case nil:
			continue
		default:
			// Unexpected error bail out
			expvarsUnexpectedItemDrops.Add(1)
			tlmUnexpectedItemDrops.Inc()
			return nil, err
		}
	}

	// if the last payload has any data, flush it
	err = finishPayload()
	if err != nil {
		return nil, err
	}

	return payloads, nil
}

// MarshalJSON serializes timeseries to JSON so it can be sent to V1 endpoints
// FIXME(maxime): to be removed when v2 endpoints are available
func (series *IterableSeries) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing a Series
	type SeriesAlias Series

	seriesAlias := make(SeriesAlias, 0)
	for series.MoveNext() {
		serie := series.source.Current()
		serie.PopulateDeviceField()
		serie.PopulateResources()
		seriesAlias = append(seriesAlias, serie)
	}

	data := map[string][]*metrics.Serie{
		"series": seriesAlias,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// SplitPayload breaks the payload into, at least, "times" number of pieces
func (series *IterableSeries) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	seriesExpvar.Add("TimesSplit", 1)
	tlmSeries.Inc("times_split")

	// We need to split series without splitting metrics across multiple
	// payload. So we first group series by metric name.
	metricsPerName := map[string]Series{}
	serieCount := 0
	for series.MoveNext() {
		s := series.source.Current()
		serieCount++
		metricsPerName[s.Name] = append(metricsPerName[s.Name], s)
	}

	// if we only have one metric name we cannot split further
	if len(metricsPerName) == 1 {
		seriesExpvar.Add("SplitMetricsTooBig", 1)
		tlmSeries.Inc("split_metrics_too_big")
		var metricName string
		for k := range metricsPerName {
			metricName = k
		}
		return nil, fmt.Errorf("Cannot split metric '%s' into %d payload (it contains %d series)", metricName, times, serieCount)
	}

	nbSeriesPerPayload := serieCount / times

	payloads := []marshaler.AbstractMarshaler{}
	current := Series{}
	for _, m := range metricsPerName {
		// If on metric is bigger than the targeted size we directly
		// add it as a payload.
		if len(m) >= nbSeriesPerPayload {
			payloads = append(payloads, m)
			continue
		}

		// Then either append to the current payload if "m" is small
		// enough or flush the current payload and start a new one.
		// This may result in more than twice the number of payloads
		// asked for but is "good enough" and will loop only once
		// through metricsPerName
		if len(current)+len(m) < nbSeriesPerPayload {
			current = append(current, m...)
		} else {
			payloads = append(payloads, current)
			current = m
		}
	}
	if len(current) != 0 {
		payloads = append(payloads, current)
	}
	return payloads, nil
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
