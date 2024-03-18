// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(APM) Fix revive linter
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
	PayloadAggregationKey
	BucketsAggregationKey BucketAggregator
}

// BucketsAggregationKey specifies the key by which a bucket is aggregated.
type BucketsAggregationKey struct {
	service      string
	name         string
	resource     string
	typ          string
	spanKind     string
	statusCode   uint32
	synthetics   bool
	peerTagsHash uint64
}

func (b *BucketsAggregationKey) Service() string {
	return b.service
}

func (b *BucketsAggregationKey) Name() string {
	return b.name
}

func (b *BucketsAggregationKey) Resource() string {
	return b.resource
}

func (b *BucketsAggregationKey) HttpStatusCode() uint32 {
	return b.statusCode
}

func (b *BucketsAggregationKey) Type() string {
	return b.typ
}

func (b *BucketsAggregationKey) Synthetics() bool {
	return b.synthetics
}

func (b *BucketsAggregationKey) SpanKind() string {
	return b.spanKind
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
func NewAggregationFromSpan(s StattableSpan, aggKey PayloadAggregationKey, enablePeerTagsAgg bool, peerTagKeys []string) Aggregation {
	agg := Aggregation{
		PayloadAggregationKey: aggKey,
		BucketsAggregationKey: s.BucketAggregationKey(),
	}
	// todo: calc these
	//var peerTags []string
	//if clientOrProducer(agg.SpanKind) && enablePeerTagsAgg {
	//	peerTags = matchingPeerTags(s, peerTagKeys)
	//	agg.PeerTagsHash = peerTagsHash(peerTags) //TODO: what do
	//}
	return agg
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
		BucketsAggregationKey: &BucketsAggregationKey{
			resource:     g.Resource,
			service:      g.Service,
			name:         g.Name,
			spanKind:     g.SpanKind,
			statusCode:   g.HTTPStatusCode,
			synthetics:   g.Synthetics,
			peerTagsHash: peerTagsHash(g.PeerTags),
		},
	}
}
