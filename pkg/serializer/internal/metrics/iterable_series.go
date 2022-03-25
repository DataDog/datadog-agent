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

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// IterableSeries is a serializer for metrics.IterableSeries
type IterableSeries struct {
	*metrics.IterableSeries
}

// WriteHeader writes the payload header for this type
func (series IterableSeries) WriteHeader(stream *jsoniter.Stream) error {
	return writeHeader(stream)
}

// WriteFooter writes the payload footer for this type
func (series IterableSeries) WriteFooter(stream *jsoniter.Stream) error {
	return writeFooter(stream)
}

// WriteCurrentItem writes the json representation of an item
func (series IterableSeries) WriteCurrentItem(stream *jsoniter.Stream) error {
	current := series.Current()
	if current == nil {
		return errors.New("nil serie")
	}
	return writeItem(stream, current)
}

// DescribeCurrentItem returns a text description for logs
func (series IterableSeries) DescribeCurrentItem() string {
	current := series.Current()
	if current == nil {
		return "nil serie"
	}
	return describeItem(current)
}

// MarshalSplitCompress uses the stream compressor to marshal and compress series payloads.
// If a compressed payload is larger than the max, a new payload will be generated. This method returns a slice of
// compressed protobuf marshaled MetricPayload objects.
func (series IterableSeries) MarshalSplitCompress(bufferContext *marshaler.BufferContext) ([]*[]byte, error) {
	return marshalSplitCompress(series, bufferContext)
}

// MarshalJSON serializes timeseries to JSON so it can be sent to V1 endpoints
//FIXME(maxime): to be removed when v2 endpoints are available
func (series IterableSeries) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing a Series
	type SeriesAlias Series

	var seriesAlias SeriesAlias
	for series.MoveNext() {
		serie := series.Current()
		serie.PopulateDeviceField()
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
func (series IterableSeries) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	seriesExpvar.Add("TimesSplit", 1)
	tlmSeries.Inc("times_split")

	// We need to split series without splitting metrics across multiple
	// payload. So we first group series by metric name.
	metricsPerName := map[string]Series{}
	serieCount := 0
	for series.MoveNext() {
		s := series.Current()
		serieCount++
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
		var metricName string
		for k, _ := range metricsPerName {
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
