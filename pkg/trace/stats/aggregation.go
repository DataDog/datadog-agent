// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package stats contains the logic to process APM stats.
package stats

import (
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const (
	tagStatusCode = "http.status_code"
	tagSynthetics = "synthetics"
	tagSpanKind   = "span.kind"
)

// Aggregation contains all the dimension on which we aggregate statistics.
type Aggregation struct {
	BucketsAggregationKey
	PayloadAggregationKey
}

// BucketsAggregationKey specifies the key by which a bucket is aggregated.
type BucketsAggregationKey struct {
	Service      string
	Name         string
	Resource     string
	Type         string
	SpanKind     string
	StatusCode   uint32
	Synthetics   bool
	PeerTagsHash uint64
	IsTraceRoot  pb.TraceRootFlag
}

// PayloadAggregationKey specifies the key by which a payload is aggregated.
type PayloadAggregationKey struct {
	Env          string
	Hostname     string
	Version      string
	ContainerID  string
	GitCommitSha string
	ImageTag     string
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

func clientOrProducer(spanKind string) bool {
	sk := strings.ToLower(spanKind)
	return sk == "client" || sk == "producer"
}

// NewAggregationFromSpan creates a new aggregation from the provided span and env
func NewAggregationFromSpan(s *pb.Span, origin string, aggKey PayloadAggregationKey, enablePeerTagsAgg bool, peerTagKeys []string) (Aggregation, []string) {
	synthetics := strings.HasPrefix(origin, tagSynthetics)
	var isTraceRoot pb.TraceRootFlag
	if s.ParentID == 0 {
		isTraceRoot = pb.TraceRootFlag_TRUE
	} else {
		isTraceRoot = pb.TraceRootFlag_FALSE
	}
	agg := Aggregation{
		PayloadAggregationKey: aggKey,
		BucketsAggregationKey: BucketsAggregationKey{
			Resource:    s.Resource,
			Service:     s.Service,
			Name:        s.Name,
			SpanKind:    s.Meta[tagSpanKind],
			Type:        s.Type,
			StatusCode:  getStatusCode(s),
			Synthetics:  synthetics,
			IsTraceRoot: isTraceRoot,
		},
	}
	var peerTags []string
	if clientOrProducer(agg.SpanKind) && enablePeerTagsAgg {
		peerTags = matchingPeerTags(s, peerTagKeys)
		agg.PeerTagsHash = peerTagsHash(peerTags)
	}
	return agg, peerTags
}

func matchingPeerTags(s *pb.Span, peerTagKeys []string) []string {
	if len(peerTagKeys) == 0 {
		return nil
	}
	var pt []string
	for _, t := range peerTagKeys {
		if v, ok := s.Meta[t]; ok && v != "" {
			pt = append(pt, t+":"+v)
		}
	}
	return pt
}

func peerTagsHash(tags []string) uint64 {
	if len(tags) == 0 {
		return 0
	}
	if !sort.StringsAreSorted(tags) {
		sort.Strings(tags)
	}
	h := fnv.New64a()
	for i, t := range tags {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(t))
	}
	return h.Sum64()
}

// NewAggregationFromGroup gets the Aggregation key of grouped stats.
func NewAggregationFromGroup(g *pb.ClientGroupedStats) Aggregation {
	return Aggregation{
		BucketsAggregationKey: BucketsAggregationKey{
			Resource:     g.Resource,
			Service:      g.Service,
			Name:         g.Name,
			SpanKind:     g.SpanKind,
			StatusCode:   g.HTTPStatusCode,
			Synthetics:   g.Synthetics,
			PeerTagsHash: peerTagsHash(g.PeerTags),
			IsTraceRoot:  g.IsTraceRoot,
		},
	}
}
