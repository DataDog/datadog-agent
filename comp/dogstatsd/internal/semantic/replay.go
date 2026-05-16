// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package semantic provides deterministic replay primitives for canonical
// DogStatsD metric records.
package semantic

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/lookback"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/seriesstats"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// CorpusVersion is the current semantic replay corpus version.
const CorpusVersion = 1

// RawMetric is a parser-level metric used to demonstrate the boundary between
// raw replay and semantic replay. Raw replay re-applies enrichment to this
// shape; semantic replay persists the resulting Record.
type RawMetric struct {
	Name       string
	Tags       []string
	Host       string
	Origin     string
	ListenerID string
	Type       metrics.MetricType
	Timestamp  time.Time
	Value      float64
}

// Descriptor is the canonical semantic identity and display metadata captured
// for deterministic replay.
type Descriptor struct {
	Key          ckey.ContextKey
	Name         string
	Host         string
	Tags         []string
	DebugViewKey string
	ListenerID   string
	Origin       string
}

// Record is one canonical metric observation after parse and enrichment.
type Record struct {
	Descriptor Descriptor
	Type       metrics.MetricType
	Timestamp  time.Time
	Value      float64
}

// Corpus is a deterministic semantic replay input.
type Corpus struct {
	Version int
	Records []Record
}

// Enricher builds semantic descriptors from raw metrics and current enrichment state.
type Enricher interface {
	Descriptor(raw RawMetric) Descriptor
}

// Sink consumes semantic records during replay.
type Sink interface {
	ObserveSemantic(record Record) error
}

// BuildRecord captures one raw metric as a canonical semantic record.
func BuildRecord(raw RawMetric, enricher Enricher) Record {
	descriptor := enricher.Descriptor(raw)
	return Record{
		Descriptor: descriptor,
		Type:       raw.Type,
		Timestamp:  raw.Timestamp,
		Value:      raw.Value,
	}
}

// Replay replays a semantic corpus into sink without parsing or re-enriching.
func Replay(corpus Corpus, sink Sink) error {
	for _, record := range corpus.Records {
		if err := sink.ObserveSemantic(record); err != nil {
			return err
		}
	}
	return nil
}

// Projection replays semantic records into the shared debug and lookback view substrates.
type Projection struct {
	SeriesStats *seriesstats.Store
	Lookback    *lookback.Store
}

// NewProjection returns a semantic replay projection over reusable materialized-view stores.
func NewProjection(series *seriesstats.Store, recent *lookback.Store) *Projection {
	return &Projection{SeriesStats: series, Lookback: recent}
}

// ObserveSemantic applies one semantic record to configured projection stores.
func (p *Projection) ObserveSemantic(record Record) error {
	if p.SeriesStats != nil {
		p.SeriesStats.Observe(record.Timestamp, seriesstats.Point{
			Key:  record.Descriptor.Key,
			Name: record.Descriptor.Name,
			Tags: strings.Join(record.Descriptor.Tags, ","),
		})
	}
	if p.Lookback != nil {
		p.Lookback.Observe(record.Timestamp, lookback.Point{
			Key:          record.Descriptor.Key,
			Name:         record.Descriptor.Name,
			DebugViewKey: record.Descriptor.DebugViewKey,
			ListenerID:   record.Descriptor.ListenerID,
			Origin:       record.Descriptor.Origin,
		})
	}
	return nil
}
