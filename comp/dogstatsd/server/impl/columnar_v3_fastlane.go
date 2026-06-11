// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"bytes"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func columnarV3FastLaneEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_FASTLANE"))
	return err == nil && enabled
}

func columnarV3FastLaneSize() int {
	if raw := os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_FASTLANE_SIZE"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil {
			return value
		}
	}
	return 65536
}

type columnarV3FastMetricSample struct {
	rawName    []byte
	value      float64
	metricType metricType
	mtype      metrics.MetricType
	sampleRate float64
	tagset     parserTagset
}

type columnarV3FastDescriptorGroup struct {
	tagsetID   uint64
	metricType metrics.MetricType
}

type columnarV3FastDescriptorKey struct {
	group columnarV3FastDescriptorGroup
	name  string
}

type columnarV3FastDescriptor struct {
	name    string
	tags    []string
	host    string
	unit    string
	source  metrics.MetricSource
	mtype   metrics.MetricType
	context identity.HotPathContext
}

type columnarV3FastDescriptorCache struct {
	maxSize int
	entries map[columnarV3FastDescriptorGroup]map[string]*columnarV3FastDescriptor
	ring    []columnarV3FastDescriptorKey
	next    int
}

func newColumnarV3FastDescriptorCache(maxSize int) *columnarV3FastDescriptorCache {
	if !columnarV3FastLaneEnabled() || maxSize <= 0 {
		return nil
	}
	return &columnarV3FastDescriptorCache{
		maxSize: maxSize,
		entries: make(map[columnarV3FastDescriptorGroup]map[string]*columnarV3FastDescriptor),
		ring:    make([]columnarV3FastDescriptorKey, maxSize),
	}
}

func (c *columnarV3FastDescriptorCache) lookup(rawName []byte, tagsetID uint64, mtype metrics.MetricType) (*columnarV3FastDescriptor, bool) {
	if c == nil || len(rawName) == 0 || tagsetID == 0 {
		return nil, false
	}
	byName := c.entries[columnarV3FastDescriptorGroup{tagsetID: tagsetID, metricType: mtype}]
	if byName == nil {
		return nil, false
	}
	// Keep string(rawName) directly in the map lookup expression so descriptor
	// hits do not allocate a temporary string.
	desc, found := byName[string(rawName)]
	return desc, found
}

func (c *columnarV3FastDescriptorCache) insert(rawName []byte, tagsetID uint64, mtype metrics.MetricType, desc *columnarV3FastDescriptor) {
	if c == nil || len(rawName) == 0 || tagsetID == 0 || desc == nil {
		return
	}
	group := columnarV3FastDescriptorGroup{tagsetID: tagsetID, metricType: mtype}
	byName := c.entries[group]
	if byName == nil {
		byName = make(map[string]*columnarV3FastDescriptor)
		c.entries[group] = byName
	}
	keyName := string(rawName)
	if _, found := byName[keyName]; found {
		return
	}

	evicted := c.ring[c.next]
	if evicted.name != "" {
		if evictedByName := c.entries[evicted.group]; evictedByName != nil {
			delete(evictedByName, evicted.name)
			if len(evictedByName) == 0 {
				delete(c.entries, evicted.group)
			}
		}
	}

	byName[keyName] = desc
	c.ring[c.next] = columnarV3FastDescriptorKey{group: group, name: keyName}
	c.next = (c.next + 1) % len(c.ring)
}

func (p *parser) parseMetricSampleColumnarV3FastLane(message []byte) (columnarV3FastMetricSample, bool) {
	if p == nil || p.tagsetInterner == nil || p.columnarV3FastDescriptors == nil {
		return columnarV3FastMetricSample{}, false
	}
	if !hasMetricSampleFormat(message) {
		return columnarV3FastMetricSample{}, false
	}

	rawNameAndValue, message := nextField(message)
	name, rawValue, err := parseMetricSampleNameAndRawValue(rawNameAndValue)
	if err != nil {
		return columnarV3FastMetricSample{}, false
	}
	if bytes.Contains(rawValue, colonSeparator) {
		return columnarV3FastMetricSample{}, false
	}

	rawMetricType, message := nextField(message)
	metricType, err := parseMetricSampleMetricType(rawMetricType)
	if err != nil {
		return columnarV3FastMetricSample{}, false
	}
	mtype := enrichMetricType(metricType)
	if !columnarV3DirectMetricTypeSupported(mtype) || metricType == setType {
		return columnarV3FastMetricSample{}, false
	}

	value, err := parseFloat64(rawValue)
	if err != nil {
		return columnarV3FastMetricSample{}, false
	}

	sampleRate := 1.0
	var tagset parserTagset
	for message != nil {
		var optionalField []byte
		optionalField, message = nextField(message)
		switch {
		case bytes.HasPrefix(optionalField, tagsFieldPrefix):
			var found bool
			tagset, found = p.tagsetInterner.Lookup(optionalField[1:])
			if !found || tagset.id == 0 {
				return columnarV3FastMetricSample{}, false
			}
		case bytes.HasPrefix(optionalField, sampleRateFieldPrefix):
			sampleRate, err = parseMetricSampleSampleRate(optionalField[1:])
			if err != nil {
				return columnarV3FastMetricSample{}, false
			}
		case bytes.HasPrefix(optionalField, timestampFieldPrefix):
			if p.readTimestamps {
				return columnarV3FastMetricSample{}, false
			}
		case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, localDataPrefix):
			return columnarV3FastMetricSample{}, false
		case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, externalDataPrefix):
			return columnarV3FastMetricSample{}, false
		case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, cardinalityPrefix):
			return columnarV3FastMetricSample{}, false
		}
	}
	if tagset.id == 0 {
		return columnarV3FastMetricSample{}, false
	}

	return columnarV3FastMetricSample{
		rawName:    name,
		value:      value,
		metricType: metricType,
		mtype:      mtype,
		sampleRate: sampleRate,
		tagset:     tagset,
	}, true
}

func newDogStatsDColumnarV3SampleFromFastDescriptor(desc *columnarV3FastDescriptor, value float64, sampleRate float64, includeDescriptor bool) aggregator.DogStatsDColumnarV3Sample {
	row := aggregator.DogStatsDColumnarV3Sample{
		ContextKey:   desc.context.Shard.ContextKey,
		CompactID:    desc.context.CompactID,
		CompactState: desc.context.CompactState,
		Value:        value,
		SampleRate:   sampleRate,
		Mtype:        desc.mtype,
	}
	if desc.context.CompactState != nil {
		if descriptorID, generation, ok := desc.context.CompactState.ColumnarDescriptorRef(desc.mtype); ok {
			row.DescriptorID = descriptorID
			row.DescriptorGeneration = generation
			row.HasDescriptorRef = true
			return row
		}
	}
	if includeDescriptor {
		row.Name = desc.name
		row.Tags = desc.tags
		row.Host = desc.host
		row.Unit = desc.unit
		row.Source = desc.source
		row.HasDescriptor = true
	}
	return row
}
