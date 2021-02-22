package dogstatsd

import (
	"bytes"
	"fmt"
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
}

// sanity checks a given message against the metric sample format
func hasMetricSampleFormat(message []byte) bool {
	if message == nil {
		return false
	}
	separatorCount := bytes.Count(message, fieldSeparator)
	if separatorCount < 1 || separatorCount > 3 {
		return false
	}
	return true
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
	return parseFloat64(rawSampleRate)
}
