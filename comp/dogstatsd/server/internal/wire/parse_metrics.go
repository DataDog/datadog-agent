// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wire

import (
	"bytes"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
)

// MetricType is the metric kind encoded in the wire message.
type MetricType int

const (
	Gauge MetricType = iota
	Count
	Distribution
	Histogram
	Set
	Timing
)

var (
	gaugeSymbol        = []byte("g")
	countSymbol        = []byte("c")
	histogramSymbol    = []byte("h")
	distributionSymbol = []byte("d")
	setSymbol          = []byte("s")
	timingSymbol       = []byte("ms")

	tagsFieldPrefix       = []byte("#")
	sampleRateFieldPrefix = []byte("@")
	timestampFieldPrefix  = []byte("T")
)

// MetricSample contains parsed fields before server enrichment.
type MetricSample struct {
	Name string
	// use for single value messages
	Value float64
	// Values borrows numeric storage until the caller returns it to the shared pool.
	Values []float64
	// use to store set's values
	SetValue   string
	Type       MetricType
	SampleRate float64
	Tags       []string
	// localData is used for Origin Detection
	LocalData origindetection.LocalData
	// externalData is used for Origin Detection
	ExternalData origindetection.ExternalData
	// cardinality is used for Origin Detection
	Cardinality string
	// timestamp read in the message if any
	Timestamp time.Time
}

// sanity checks a given message against the metric sample format
func hasMetricSampleFormat(message []byte) bool {
	if message == nil {
		return false
	}
	separatorCount := bytes.Count(message, fieldSeparator)
	return separatorCount >= 1
}

func parseMetricSampleNameAndRawValue(rawNameAndValue []byte) ([]byte, []byte, error) {
	sepIndex := bytes.Index(rawNameAndValue, colonSeparator)
	if sepIndex == -1 {
		return nil, nil, fmt.Errorf("invalid name and value: %q", rawNameAndValue)
	}
	rawName := rawNameAndValue[:sepIndex]
	rawValue := rawNameAndValue[sepIndex+1:]
	if len(rawName) == 0 || len(rawValue) == 0 {
		return nil, nil, fmt.Errorf("invalid name and value: %q", rawNameAndValue)
	}
	return rawName, rawValue, nil
}

func parseMetricSampleMetricType(rawMetricType []byte) (MetricType, error) {
	switch {
	case bytes.Equal(rawMetricType, gaugeSymbol):
		return Gauge, nil
	case bytes.Equal(rawMetricType, countSymbol):
		return Count, nil
	case bytes.Equal(rawMetricType, histogramSymbol):
		return Histogram, nil
	case bytes.Equal(rawMetricType, distributionSymbol):
		return Distribution, nil
	case bytes.Equal(rawMetricType, setSymbol):
		return Set, nil
	case bytes.Equal(rawMetricType, timingSymbol):
		return Timing, nil
	}
	return 0, fmt.Errorf("invalid metric type: %q", rawMetricType)
}

func parseMetricSampleSampleRate(rawSampleRate []byte) (float64, error) {
	return parseFloat64(rawSampleRate)
}
