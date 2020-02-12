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
	name       string
	value      float64
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

func (p *parser) parseMetricSample(message []byte) (dogstatsdMetricSample, error) {
	// fast path to eliminate most of the gibberish
	// especially important here since all the unidentified garbage gets
	// identified as metrics
	if !hasMetricSampleFormat(message) {
		return dogstatsdMetricSample{}, fmt.Errorf("invalid dogstatsd message format")
	}

	rawNameAndValue, message := nextField(message)
	name, rawValue, err := parseMetricSampleNameAndRawValue(rawNameAndValue)
	if err != nil {
		return dogstatsdMetricSample{}, err
	}

	rawMetricType, message := nextField(message)
	metricType, err := parseMetricSampleMetricType(rawMetricType)
	if err != nil {
		return dogstatsdMetricSample{}, err
	}

	var setValue []byte
	var value float64
	if metricType == setType {
		setValue = rawValue
	} else {
		value, err = parseFloat64(rawValue)
		if err != nil {
			return dogstatsdMetricSample{}, fmt.Errorf("could not parse dogstatsd metric value: %v", err)
		}
	}

	sampleRate := 1.0
	var tags []string
	var optionalField []byte
	for message != nil {
		optionalField, message = nextField(message)
		if bytes.HasPrefix(optionalField, tagsFieldPrefix) {
			tags = p.parseTags(optionalField[1:])
		} else if bytes.HasPrefix(optionalField, sampleRateFieldPrefix) {
			sampleRate, err = parseMetricSampleSampleRate(optionalField[1:])
			if err != nil {
				return dogstatsdMetricSample{}, fmt.Errorf("could not parse dogstatsd sample rate %q", optionalField)
			}
		}
	}

	return dogstatsdMetricSample{
		name:       p.interner.LoadOrStore(name),
		value:      value,
		setValue:   string(setValue),
		metricType: metricType,
		sampleRate: sampleRate,
		tags:       tags,
	}, nil
}
