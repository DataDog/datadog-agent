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
	allSpanNames   = "*"
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

type CustomTagKey string

func NewCustomTagKey(customTags []string) CustomTagKey {
	var tags []string

	for _, tag := range customTags {
		tags = append(tags, strings.TrimSpace(tag))
	}
	return CustomTagKey(strings.Join(tags, ","))
}

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
func NewAggregationFromSpan(s *pb.Span, origin string, aggKey PayloadAggregationKey, enablePeerSvcAgg bool, customTagConf map[string][]string) Aggregation {
	synthetics := strings.HasPrefix(origin, tagSynthetics)

	// log.Info("conf map: ")
	// log.Info(customTagConf)
	customTagSlice := []string{}

	spanTags, ok := customTagConf[s.Name]

	if ok {
		for k := range spanTags {
			tags, ok := s.Meta[spanTags[k]]

			if ok {
				customTagSlice = append(customTagSlice, spanTags[k]+":"+tags)
			}
		}
	}

	AllSpanNameTags, ok := customTagConf[allSpanNames]

	if ok {
		for k := range AllSpanNameTags {
			tags, ok := s.Meta[AllSpanNameTags[k]]

			if ok {
				customTagSlice = append(customTagSlice, AllSpanNameTags[k]+":"+tags)
			}
		}
	}
	customKey := NewCustomTagKey(customTagSlice)
	log.Info("tag key: " + customKey)

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
		CustomTagKey: CustomTagKey(g.CustomTags),
	}
}
