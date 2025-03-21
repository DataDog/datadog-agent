// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"bytes"
	"fmt"
	"strconv"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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

	// containerIDFieldPrefix is the prefix for a common field holding the sender's container ID
	containerIDFieldPrefix = []byte("c:")

	// containerInodeFieldPrefix is the prefix for a notation holding the sender's container Inode in the containerIDField
	containerIDFieldInodePrefix = []byte("in-")
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

func newParser(cfg config.Reader, float64List *float64ListPool, workerNum int, wmeta optional.Option[workloadmeta.Component]) *parser {
	stringInternerCacheSize := cfg.GetInt("dogstatsd_string_interner_size")
	readTimestamps := cfg.GetBool("dogstatsd_no_aggregation_pipeline")

	return &parser{
		interner:         newStringInterner(stringInternerCacheSize, workerNum),
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
	var containerID []byte
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
				return dogstatsdMetricSample{}, fmt.Errorf("dogstatsd timestamp should be > 0")
			}
			timestamp = time.Unix(ts, 0)
		// container ID
		case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, containerIDFieldPrefix):
			containerID = p.extractContainerID(optionalField)
		}
	}

	return dogstatsdMetricSample{
		name:        p.interner.LoadOrStore(name),
		value:       value,
		values:      values,
		setValue:    string(setValue),
		metricType:  metricType,
		sampleRate:  sampleRate,
		tags:        tags,
		containerID: containerID,
		ts:          timestamp,
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

// extractContainerID parses the value of the container ID field.
// If the field is prefixed by `in-`, it corresponds to the cgroup controller's inode of the source
// and is used for ContainerID resolution.
func (p *parser) extractContainerID(rawContainerIDField []byte) []byte {
	containerIDField := rawContainerIDField[len(containerIDFieldPrefix):]

	if bytes.HasPrefix(containerIDField[:len(containerIDFieldInodePrefix)], containerIDFieldInodePrefix) {
		inodeField, err := strconv.ParseUint(string(containerIDField[len(containerIDFieldPrefix)+1:]), 10, 64)
		if err != nil {
			log.Debugf("Failed to parse inode from %s, got %v", containerIDField, err)
			return nil
		}

		containerID, err := p.provider.GetMetaCollector().GetContainerIDForInode(inodeField, cacheValidity)
		if err != nil {
			log.Debugf("Failed to get container ID, got %v", err)
			return nil
		}
		return []byte(containerID)
	}

	return containerIDField
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
