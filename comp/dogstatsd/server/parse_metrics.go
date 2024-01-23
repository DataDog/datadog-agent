// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"bytes"
	"fmt"
	"time"
)

type metricType int

const (
	gaugeType metricType = iota
	countType
	distributionType
	histogramType
	setType
	timingType
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

type dogstatsdMetricSample struct {
	name string
	// use for single value messages
	value float64
	// use for multiple value messages
	values []float64
	// use to store set's values
	setValue   string
	metricType metricType
	sampleRate float64
	tags       []string
	// containerID represents the container ID of the sender (optional).
	containerID []byte
	// timestamp read in the message if any
	ts time.Time
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

func parseMetricSampleMetricType(rawMetricType []byte) (metricType, error) {
	switch {
	case bytes.Equal(rawMetricType, gaugeSymbol):
		return gaugeType, nil
	case bytes.Equal(rawMetricType, countSymbol):
		return countType, nil
	case bytes.Equal(rawMetricType, histogramSymbol):
		return histogramType, nil
	case bytes.Equal(rawMetricType, distributionSymbol):
		return distributionType, nil
	case bytes.Equal(rawMetricType, setSymbol):
		return setType, nil
	case bytes.Equal(rawMetricType, timingSymbol):
		return timingType, nil
	}
	return 0, fmt.Errorf("invalid metric type: %q", rawMetricType)
}

func parseMetricSampleSampleRate(rawSampleRate []byte) (float64, error) {
	panic("not called")
}
