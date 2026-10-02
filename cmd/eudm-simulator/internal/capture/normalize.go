// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package capture makes owned copies of observed telemetry for portable replay.
package capture

import (
	"encoding/json"
	"errors"
	"slices"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Normalizer preserves observed values. The live coordinator makes collection
// timestamps relative; replay assigns simulated device identities later.
type Normalizer struct{}

func NewNormalizer() *Normalizer { return &Normalizer{} }

// SeriesSource is an owned, finite copy of a serializer source.
type SeriesSource struct {
	Series []*metrics.Serie
	index  int
}

func NewSeriesSource(series []*metrics.Serie) *SeriesSource {
	return &SeriesSource{Series: series, index: -1}
}
func (s *SeriesSource) MoveNext() bool          { s.index++; return s.index < len(s.Series) }
func (s *SeriesSource) Current() *metrics.Serie { return s.Series[s.index] }
func (s *SeriesSource) Count() uint64           { return uint64(len(s.Series)) }

// Series copies all observed metric fields without changing their values.
func (*Normalizer) Series(source metrics.SerieSource) (metrics.SerieSource, error) {
	var result []*metrics.Serie
	for source.MoveNext() {
		value := source.Current()
		if value == nil {
			return nil, errors.New("invalid nil metric series")
		}
		owned := *value
		owned.Points = slices.Clone(value.Points)
		owned.Resources = slices.Clone(value.Resources)
		owned.Tags = tagset.CompositeTagsFromSlice(slices.Clone(value.Tags.UnsafeToReadOnlySliceString()))
		result = append(result, &owned)
	}
	return NewSeriesSource(result), nil
}

// HostMetadata has no credential or Agent-configuration fields. Producers
// construct this explicit projection before any data crosses capture IPC.
type HostMetadata = telemetry.HostMetadata

func (*Normalizer) HostMetadata(native marshaler.JSONMarshaler) (marshaler.JSONMarshaler, error) {
	data, err := native.MarshalJSON()
	if err != nil {
		return nil, errors.New("cannot read native host metadata")
	}
	var owned HostMetadata
	if json.Unmarshal(data, &owned) != nil {
		return nil, errors.New("invalid native host metadata")
	}
	if owned.Gohai != "" && !json.Valid([]byte(owned.Gohai)) {
		return nil, errors.New("invalid native gohai metadata")
	}
	return &owned, nil
}

// Process retains the complete Agent-scrubbed message, including process
// arguments, users, I/O counters, and fields added to the protobuf contract.
func (*Normalizer) Process(in *model.CollectorProc) *model.CollectorProc {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*model.CollectorProc)
}

// Connections retains the full payload, including encoded DNS and tag tables.
// Validate DNS framing before an iterator can encounter malformed input; no
// decoded table or identifier rewrite is needed to preserve the native bytes.
func (*Normalizer) Connections(in *model.CollectorConnections) (out *model.CollectorConnections) {
	if in == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			out = nil
		}
	}()
	if len(in.EncodedDnsLookups) > 0 && telemetry.ValidateConnectionDNSV2(in.EncodedDomainDatabase, in.EncodedDnsLookups) != nil {
		return nil
	}
	if len(in.EncodedDNS) > 0 {
		for _, connection := range in.Connections {
			if connection == nil {
				continue
			}
			for _, address := range []*model.Addr{connection.Laddr, connection.Raddr} {
				if address != nil {
					if err := in.IterateDNS(address, func(_, _ int, _ string) bool { return true }); err != nil {
						return nil
					}
				}
			}
		}
	}
	return proto.Clone(in).(*model.CollectorConnections)
}
