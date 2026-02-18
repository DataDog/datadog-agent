// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type messageType int

const (
	metricSampleType messageType = iota
	serviceCheckType
	eventType
	cacheValidity = 2 * time.Second
)

var (
	eventPrefix        = []byte("_e{")
	serviceCheckPrefix = []byte("_sc")

	fieldSeparator = []byte("|")
	colonSeparator = []byte(":")
	commaSeparator = []byte(",")

	// localDataPrefix is the prefix for a common field which contains the local data for Origin Detection.
	// The Local Data is a list that can contain one or two (split by a ',') of either:
	// * "cid-<container-id>" or "ci-<container-id>" for the container ID.
	// * "in-<cgroupv2-inode>" for the cgroupv2 inode.
	// Possible values:
	// * "cid-<container-id>"
	// * "ci-<container-id>,in-<cgroupv2-inode>"
	localDataPrefix = []byte("c:")

	// externalDataPrefix is the prefix for a common field which contains the external data for Origin Detection.
	externalDataPrefix = []byte("e:")

	// cardinalityPrefix is the prefix for a common field which contains the cardinality for Origin Detection.
	cardinalityPrefix = []byte("card:")
)

// parser parses dogstatsd messages
// not safe for concurent use
type parser struct {
	interner    *stringInterner
	float64List *float64ListPool

	// dsdOriginEnabled controls whether the server should honor the container id sent by the
	// client. Defaulting to false, this opt-in flag is used to avoid changing tags cardinality
	// for existing installations.
	dsdOriginEnabled bool

	// readTimestamps is true if the parser has to read timestamps from messages.
	readTimestamps bool

	// Generic Metric Provider
	provider provider.Provider
}

func newParser(cfg model.Reader, float64List *float64ListPool, workerNum int, wmeta option.Option[workloadmeta.Component], stringInternerTelemetry *stringInternerTelemetry) *parser {
	stringInternerCacheSize := cfg.GetInt("dogstatsd_string_interner_size")
	readTimestamps := cfg.GetBool("dogstatsd_no_aggregation_pipeline")

	return &parser{
		interner:         newStringInterner(stringInternerCacheSize, workerNum, stringInternerTelemetry),
		readTimestamps:   readTimestamps,
		float64List:      float64List,
		dsdOriginEnabled: cfg.GetBool("dogstatsd_origin_detection_client"),
		provider:         provider.GetProvider(wmeta),
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

func (p *parser) parseTags(rawTags []byte) []string {
	if len(rawTags) == 0 {
		return nil
	}
	tagsCount := bytes.Count(rawTags, commaSeparator)
	tagsList := make([]string, tagsCount+1)

	i := 0
	for i < tagsCount {
		tagPos := bytes.Index(rawTags, commaSeparator)
		if tagPos < 0 {
			break
		}
		tagsList[i] = p.interner.LoadOrStore(rawTags[:tagPos])
		rawTags = rawTags[tagPos+len(commaSeparator):]
		i++
	}
	tagsList[i] = p.interner.LoadOrStore(rawTags)
	return tagsList
}

// parseMetricSample parses the given message and return the dogstatsdMetricSample read.
func (p *parser) parseMetricSample(message []byte) (dogstatsdMetricSample, error) {
	// fast path to eliminate most of the gibberish
	// especially important here since all the unidentified garbage gets
	// identified as metrics
	if !hasMetricSampleFormat(message) {
		return dogstatsdMetricSample{}, errors.New("invalid dogstatsd message format")
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

	// read metric values

	var setValue []byte
	var values []float64
	var value float64
	if metricType == setType {
		setValue = rawValue // special case for the set type, we obviously don't support multiple values for this type
	} else {
		// In case the list contains only one value, dogstatsd 1.0
		// protocol, we directly parse it as a float64. This avoids
		// pulling a slice from the float64List and greatly improve
		// performances.
		if !bytes.Contains(rawValue, colonSeparator) {
			value, err = parseFloat64(rawValue)
		} else {
			values, err = p.parseFloat64List(rawValue)
		}
		if err != nil {
			return dogstatsdMetricSample{}, fmt.Errorf("could not parse dogstatsd metric values: %v", err)
		}
	}

	// now, look for extra fields supported by dogstatsd
	// sample rate, tags, container ID, timestamp, ...

	sampleRate := 1.0
	var tags []string
	var localData origindetection.LocalData
	var externalData origindetection.ExternalData
	var cardinality string
	var optionalField []byte
	var timestamp time.Time
	for message != nil {
		optionalField, message = nextField(message)
		switch {
		// tags
		case bytes.HasPrefix(optionalField, tagsFieldPrefix):
			tags = p.parseTags(optionalField[1:])
		// sample rate
		case bytes.HasPrefix(optionalField, sampleRateFieldPrefix):
			sampleRate, err = parseMetricSampleSampleRate(optionalField[1:])
			if err != nil {
				return dogstatsdMetricSample{}, fmt.Errorf("could not parse dogstatsd sample rate %q", optionalField)
			}
		// timestamp
		case bytes.HasPrefix(optionalField, timestampFieldPrefix):
			if !p.readTimestamps {
				continue
			}
			ts, err := strconv.ParseInt(string(optionalField[len(timestampFieldPrefix):]), 10, 0)
			if err != nil {
				return dogstatsdMetricSample{}, fmt.Errorf("could not parse dogstatsd timestamp %q: %v", optionalField[len(timestampFieldPrefix):], err)
			}
			if ts < 1 {
				return dogstatsdMetricSample{}, errors.New("dogstatsd timestamp should be > 0")
			}
			timestamp = time.Unix(ts, 0)
		// local data
		case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, localDataPrefix):
			localData = p.parseLocalData(optionalField[len(localDataPrefix):])
		// external data
		case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, externalDataPrefix):
			externalData = p.parseExternalData(optionalField[len(externalDataPrefix):])
		// cardinality
		case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, cardinalityPrefix):
			cardinality = string(optionalField[len(cardinalityPrefix):])
		}
	}

	return dogstatsdMetricSample{
		name:         p.interner.LoadOrStore(name),
		value:        value,
		values:       values,
		setValue:     string(setValue),
		metricType:   metricType,
		sampleRate:   sampleRate,
		tags:         tags,
		localData:    localData,
		externalData: externalData,
		cardinality:  cardinality,
		ts:           timestamp,
	}, nil
}

// parseLocalData parses the local data string into a LocalData struct.
func (p *parser) parseLocalData(rawLocalData []byte) origindetection.LocalData {
	localDataString := string(rawLocalData)

	localData, err := origindetection.ParseLocalData(localDataString)
	if err != nil {
		log.Errorf("failed to parse c: field containing Local Data %q: %v", localDataString, err)
	}

	// return localData even if there was a parsing error as some fields might have been parsed correctly.
	return localData
}

// parseExternalData parses the external data string into an ExternalData struct.
func (p *parser) parseExternalData(rawExternalData []byte) origindetection.ExternalData {
	externalDataString := string(rawExternalData)

	externalData, err := origindetection.ParseExternalData(externalDataString)
	if err != nil {
		log.Errorf("failed to parse e: field containing External Data %q: %v", externalDataString, err)
	}

	// return externalData even if there was a parsing error as some fields might have been parsed correctly.
	return externalData
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
		return nil, errors.New("no value found")
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

func parseInt(rawInt []byte) (int, error) {
	return strconv.Atoi(*(*string)(unsafe.Pointer(&rawInt)))
}
