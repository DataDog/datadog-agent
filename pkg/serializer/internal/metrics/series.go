// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/richardartoul/molecule"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/stream"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	seriesExpvar = expvar.NewMap("series")

	tlmSeries = telemetry.NewCounter("metrics", "series_split",
		[]string{"action"}, "Series split")
)

// Series represents a list of metrics.Serie ready to be serialize
type Series []*metrics.Serie

// populateDeviceField removes any `device:` tag in the series tags and uses the value to
// populate the Serie.Device field
//FIXME(olivier): remove this as soon as the v1 API can handle `device` as a regular tag
func populateDeviceField(serie *metrics.Serie) {
	if !hasDeviceTag(serie) {
		return
	}
	// make a copy of the tags array. Otherwise the underlying array won't have
	// the device tag for the Nth iteration (N>1), and the deice field will
	// be lost
	filteredTags := make([]string, 0, len(serie.Tags))

	for _, tag := range serie.Tags {
		if strings.HasPrefix(tag, "device:") {
			serie.Device = tag[7:]
		} else {
			filteredTags = append(filteredTags, tag)
		}
	}

	serie.Tags = filteredTags
}

// hasDeviceTag checks whether a series contains a device tag
func hasDeviceTag(serie *metrics.Serie) bool {
	for _, tag := range serie.Tags {
		if strings.HasPrefix(tag, "device:") {
			return true
		}
	}
	return false
}

// MarshalJSON serializes timeseries to JSON so it can be sent to V1 endpoints
//FIXME(maxime): to be removed when v2 endpoints are available
func (series Series) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing a Series
	type SeriesAlias Series
	for _, serie := range series {
		populateDeviceField(serie)
	}

	data := map[string][]*metrics.Serie{
		"series": SeriesAlias(series),
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// SplitPayload breaks the payload into, at least, "times" number of pieces
func (series Series) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	seriesExpvar.Add("TimesSplit", 1)
	tlmSeries.Inc("times_split")

	// We need to split series without splitting metrics across multiple
	// payload. So we first group series by metric name.
	metricsPerName := map[string]Series{}
	for _, s := range series {
		if _, ok := metricsPerName[s.Name]; ok {
			metricsPerName[s.Name] = append(metricsPerName[s.Name], s)
		} else {
			metricsPerName[s.Name] = Series{s}
		}
	}

	// if we only have one metric name we cannot split further
	if len(metricsPerName) == 1 {
		seriesExpvar.Add("SplitMetricsTooBig", 1)
		tlmSeries.Inc("split_metrics_too_big")
		return nil, fmt.Errorf("Cannot split metric '%s' into %d payload (it contains %d series)", series[0].Name, times, len(series))
	}

	nbSeriesPerPayload := len(series) / times

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

// MarshalSplitCompress uses the stream compressor to marshal and compress series payloads.
// If a compressed payload is larger than the max, a new payload will be generated. This method returns a slice of
// compressed protobuf marshaled MetricPayload objects.
func (series Series) MarshalSplitCompress(bufferContext *marshaler.BufferContext) ([]*[]byte, error) {
	return marshalSplitCompress(newSerieSliceIterator(series), bufferContext)
}

type serieIterator interface {
	MoveNext() bool
	Current() *metrics.Serie
}

var _ serieIterator = (*serieSliceIterator)(nil)

// serieSliceIterator implements serieIterator interface for `[]*metrics.Serie`.
type serieSliceIterator struct {
	series []*metrics.Serie
	index  int
}

func newSerieSliceIterator(series []*metrics.Serie) *serieSliceIterator {
	return &serieSliceIterator{
		series: series,
		index:  -1,
	}
}
func (s *serieSliceIterator) MoveNext() bool {
	s.index++
	return s.index < len(s.series)
}

func (s *serieSliceIterator) Current() *metrics.Serie {
	return s.series[s.index]
}

// MarshalSplitCompress uses the stream compressor to marshal and compress series payloads.
// If a compressed payload is larger than the max, a new payload will be generated. This method returns a slice of
// compressed protobuf marshaled MetricPayload objects.
func marshalSplitCompress(iterator serieIterator, bufferContext *marshaler.BufferContext) ([]*[]byte, error) {
	var err error
	var compressor *stream.Compressor
	buf := bufferContext.PrecompressionBuf
	ps := molecule.NewProtoStream(buf)
	payloads := []*[]byte{}

	var pointsThisPayload int
	var seriesThisPayload int
	var serie *metrics.Serie
	maxPointsPerPayload := config.Datadog.GetInt("serializer_max_series_points_per_payload")

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
	const resourceType = 1
	const resourceName = 2
	const pointValue = 1
	const pointTimestamp = 2

	// Prepare to write the next payload
	startPayload := func() error {
		var err error

		pointsThisPayload = 0
		seriesThisPayload = 0
		bufferContext.CompressorInput.Reset()
		bufferContext.CompressorOutput.Reset()

		compressor, err = stream.NewCompressor(bufferContext.CompressorInput, bufferContext.CompressorOutput, []byte{}, []byte{}, []byte{})
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
			payloads = append(payloads, &payload)
		}

		return nil
	}

	// start things off
	err = startPayload()
	if err != nil {
		return nil, err
	}

	for iterator.MoveNext() {
		serie = iterator.Current()

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

			err = ps.String(seriesMetric, serie.Name)
			if err != nil {
				return err
			}

			for _, tag := range serie.Tags {
				err = ps.String(seriesTags, tag)
				if err != nil {
					return err
				}
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

func writeHeader(stream *jsoniter.Stream) error {
	stream.WriteObjectStart()
	stream.WriteObjectField("series")
	stream.WriteArrayStart()
	return stream.Flush()
}

func writeFooter(stream *jsoniter.Stream) error {
	stream.WriteArrayEnd()
	stream.WriteObjectEnd()
	return stream.Flush()
}

func writeItem(stream *jsoniter.Stream, serie *metrics.Serie) error {
	populateDeviceField(serie)
	encodeSerie(serie, stream)
	return stream.Flush()
}

func describeItem(serie *metrics.Serie) string {
	return fmt.Sprintf("name %q, %d points", serie.Name, len(serie.Points))
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
	for _, s := range serie.Tags {
		if !firstTag {
			stream.WriteMore()
		}
		stream.WriteString(s)
		firstTag = false
	}
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

//// The following methods implement the StreamJSONMarshaler interface
//// for support of the enable_stream_payload_serialization option.

// WriteHeader writes the payload header for this type
func (series Series) WriteHeader(stream *jsoniter.Stream) error {
	return writeHeader(stream)
}

// WriteFooter writes the payload footer for this type
func (series Series) WriteFooter(stream *jsoniter.Stream) error {
	return writeFooter(stream)
}

// WriteItem writes the json representation of an item
func (series Series) WriteItem(stream *jsoniter.Stream, i int) error {
	if i < 0 || i > len(series)-1 {
		return errors.New("out of range")
	}
	return writeItem(stream, series[i])
}

// Len returns the number of items to marshal
func (series Series) Len() int {
	return len(series)
}

// DescribeItem returns a text description for logs
func (series Series) DescribeItem(i int) string {
	if i < 0 || i > len(series)-1 {
		return "out of range"
	}
	return describeItem(series[i])
}
