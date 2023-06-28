// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const (
	tagStatusCode  = "http.status_code"
	tagSynthetics  = "synthetics"
	tagPeerService = "peer.service"
)

// Aggregation contains all the dimension on which we aggregate statistics.
type Aggregation struct {
	BucketsAggregationKey
	PayloadAggregationKey
	CustomTagKey
}

// BucketsAggregationKey specifies the key by which a bucket is aggregated.
type BucketsAggregationKey struct {
	Service     string
	Name        string
	PeerService string
	Resource    string
	Type        string
	StatusCode  uint32
	Synthetics  bool
}

// PayloadAggregationKey specifies the key by which a payload is aggregated.
type PayloadAggregationKey struct {
	Env         string
	Hostname    string
	Version     string
	ContainerID string
}

type CustomTagKey struct {
	fields map[string]string
}

type CustomSpan struct {
	pb.Span
}

func NewCustomTagKey(customTags []string) *CustomTagKey {
	ctk := &CustomTagKey{
		fields: make(map[string]string),
	}

	for _, f := range customTags {
		ctk.fields[f] = ""
	}

	return ctk
}

// func (s *CustomSpan) HasTag(tag string) bool {
// 	_, exists := s.Meta[tag]
// 	return exists
// }

// func (s *CustomSpan) GetTag(tag string) (string, bool) {
// 	value, exists := s.Meta[tag]
// 	return value, exists
// }

func (ctk *CustomTagKey) AggregateTags(span *pb.Span) {
	for f := range ctk.fields {
		if value, exists := span.Meta[f]; exists {
			ctk.fields[f] = value
		}
	}

}

// customTags := []string{"georegion", "costcenter"}
// 	ctk := NewCustomTagKey(customTags)

// 	span := CustomSpan{
// 		Meta: map[string]string{
// 			"georegion":   "us-west",
// 			"costcenter":  "12345",
// 			"other_tag":   "value",
// 		},
// 	}

// 	ctk.AggregateTags(span)

func getStatusCode(s *pb.Span) uint32 {
	code, ok := traceutil.GetMetric(s, tagStatusCode)
	if ok {
		// only 7.39.0+, for lesser versions, always use Meta
		return uint32(code)
	}
	strC := traceutil.GetMetaDefault(s, tagStatusCode, "")
	if strC == "" {
		return 0
	}
	c, err := strconv.ParseUint(strC, 10, 32)
	if err != nil {
		log.Debugf("Invalid status code %s. Using 0.", strC)
		return 0
	}
	return uint32(c)
}

// NewAggregationFromSpan creates a new aggregation from the provided span and env
func NewAggregationFromSpan(s *pb.Span, origin string, aggKey PayloadAggregationKey, enablePeerSvcAgg bool, customKey CustomTagKey) Aggregation {
	synthetics := strings.HasPrefix(origin, tagSynthetics)

	// customTags := make([]string, len(s.Meta))

	// i := 0
	// for k := range s.Meta {
	// 	customTags[i] = s.Meta[k]
	// 	i++
	// }

	// customKey := NewCustomTagKey(customTags)

	agg := Aggregation{
		PayloadAggregationKey: aggKey,
		BucketsAggregationKey: BucketsAggregationKey{
			Resource:   s.Resource,
			Service:    s.Service,
			Name:       s.Name,
			Type:       s.Type,
			StatusCode: getStatusCode(s),
			Synthetics: synthetics,
		},
		CustomTagKey: customKey,
	}
	if enablePeerSvcAgg {
		agg.PeerService = s.Meta[tagPeerService]
	}
	return agg
}

// NewAggregationFromGroup gets the Aggregation key of grouped stats.
func NewAggregationFromGroup(g pb.ClientGroupedStats) Aggregation {
	return Aggregation{
		BucketsAggregationKey: BucketsAggregationKey{
			Resource:    g.Resource,
			Service:     g.Service,
			PeerService: g.PeerService,
			Name:        g.Name,
			StatusCode:  g.HTTPStatusCode,
			Synthetics:  g.Synthetics,
		},
	}
}
