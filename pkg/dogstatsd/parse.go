// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type messageType int

const (
	metricSampleType messageType = iota
	serviceCheckType
	eventType
)

// Tag prefixes with special meaning
const (
	hostTagPrefix        = "host:"
	entityIDTagPrefix    = "dd.internal.entity_id:"
	CardinalityTagPrefix = "dd.internal.card:"
)

var (
	eventPrefix        = []byte("_e{")
	serviceCheckPrefix = []byte("_sc")

	fieldSeparator = []byte("|")
	colonSeparator = []byte(":")
	commaSeparator = []byte(",")
)

// parser parses dogstatsd messages
// not safe for concurent use
type parser struct {
	interner    *stringInterner
	float64List *float64ListPool
}

func newParser(float64List *float64ListPool) *parser {
	stringInternerCacheSize := config.Datadog.GetInt("dogstatsd_string_interner_size")

	return &parser{
		interner:    newStringInterner(stringInternerCacheSize),
		float64List: float64List,
	}
}

func findMessageType(message []byte) messageType {
	if bytes.HasPrefix(message, eventPrefix) {
		return eventType
	} else if bytes.HasPrefix(message, serviceCheckPrefix) {
		return serviceCheckType
	}
	// Note that random gibberish is interpreted as a metric since they don't
	// contain any easily identifiable feature
	return metricSampleType
}

// nextField returns the data found before the first fieldSeparator and
// the remainder, as a no-heap alternative to bytes.Split.
// If the separator is not found, the remainder is nil.
func nextField(message []byte) ([]byte, []byte) {
	sepIndex := bytes.Index(message, fieldSeparator)
	if sepIndex == -1 {
		return message, nil
	}
	return message[:sepIndex], message[sepIndex+1:]
}

// TODO: this should really return a struct with the special tags broken out
func (p *parser) parseTags(rawTags []byte) (tags *tagset.Tags, hostTag, entityIDTag, cardinalityTag string) {
	tags = tagset.EmptyTags
	if len(rawTags) == 0 {
		return
	}
	tagsCount := bytes.Count(rawTags, commaSeparator)
	bldr := tagset.NewBuilder(tagsCount + 1)

	add := func(tag string) {
		if strings.HasPrefix(tag, hostTagPrefix) {
			hostTag = tag
		} else if strings.HasPrefix(tag, entityIDTagPrefix) {
			entityIDTag = tag
		} else if strings.HasPrefix(tag, CardinalityTagPrefix) {
			cardinalityTag = tag
		} else {
			bldr.Add(tag)
		}
	}

	i := 0
	for i < tagsCount {
		tagPos := bytes.Index(rawTags, commaSeparator)
		if tagPos < 0 {
			break
		}
		add(p.interner.LoadOrStore(rawTags[:tagPos]))
		rawTags = rawTags[tagPos+len(commaSeparator):]
		i++
	}
	add(p.interner.LoadOrStore(rawTags))
	tags = bldr.Close()
	return
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
	var values []float64
	var value float64
	if metricType == setType {
		setValue = rawValue
	} else {
		// In case the list contains only one value, dogstatsd 1.0
		// protocol, we directly parse it as a float64. This avoids
		// pulling a slice from the float64List and greatly improve
		// performances.
		if bytes.Index(rawValue, colonSeparator) == -1 {
			value, err = parseFloat64(rawValue)
		} else {
			values, err = p.parseFloat64List(rawValue)
		}
		if err != nil {
			return dogstatsdMetricSample{}, fmt.Errorf("could not parse dogstatsd metric values: %v", err)
		}
	}

	sampleRate := 1.0
	tags := tagset.EmptyTags
	var hostTag, entityIDTag, cardinalityTag string
	var optionalField []byte
	for message != nil {
		optionalField, message = nextField(message)
		if bytes.HasPrefix(optionalField, tagsFieldPrefix) {
			tags, hostTag, entityIDTag, cardinalityTag = p.parseTags(optionalField[1:])
		} else if bytes.HasPrefix(optionalField, sampleRateFieldPrefix) {
			sampleRate, err = parseMetricSampleSampleRate(optionalField[1:])
			if err != nil {
				return dogstatsdMetricSample{}, fmt.Errorf("could not parse dogstatsd sample rate %q", optionalField)
			}
		}
	}

	return dogstatsdMetricSample{
		name:           p.interner.LoadOrStore(name),
		value:          value,
		values:         values,
		setValue:       string(setValue),
		metricType:     metricType,
		sampleRate:     sampleRate,
		tags:           tags,
		hostTag:        hostTag,
		entityIDTag:    entityIDTag,
		cardinalityTag: cardinalityTag,
	}, nil
}

// parseFloat64List parses a list of float64 separated by colonSeparator.
func (p *parser) parseFloat64List(rawFloats []byte) ([]float64, error) {
	var value float64
	var err error
	idx := 0

	values := p.float64List.get()
	for idx != -1 && len(rawFloats) != 0 {
		idx = bytes.Index(rawFloats, colonSeparator)
		// skip empty value such as '21::22'
		if idx == 0 {
			rawFloats = rawFloats[len(colonSeparator):]
			continue
		}

		// last value
		if idx == -1 {
			value, err = parseFloat64(rawFloats)
		} else {
			value, err = parseFloat64(rawFloats[0:idx])
			rawFloats = rawFloats[idx+len(colonSeparator):]
		}

		if err != nil {
			p.float64List.put(values)
			return nil, err
		}

		values = append(values, value)
	}
	if len(values) == 0 {
		p.float64List.put(values)
		return nil, fmt.Errorf("no value found")
	}
	return values, nil
}

// the std API does not have methods to do []byte => float parsing
// we use this unsafe trick to avoid having to allocate one string for
// every parsed float
// see https://github.com/golang/go/issues/2632
func parseFloat64(rawFloat []byte) (float64, error) {
	return strconv.ParseFloat(*(*string)(unsafe.Pointer(&rawFloat)), 64)
}

// the std API does not have methods to do []byte => float parsing
// we use this unsafe trick to avoid having to allocate one string for
// every parsed float
// see https://github.com/golang/go/issues/2632
func parseInt64(rawInt []byte) (int64, error) {
	return strconv.ParseInt(*(*string)(unsafe.Pointer(&rawInt)), 10, 64)
}
