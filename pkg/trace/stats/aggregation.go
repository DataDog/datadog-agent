// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"hash/fnv"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const (
	tagStatusCode  = "http.status_code"
	tagSynthetics  = "synthetics"
	tagPeerService = "peer.service"
	tagSpanKind    = "span.kind"
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
	PeerService  string
	Resource     string
	Type         string
	SpanKind     string
	StatusCode   uint32
	Synthetics   bool
	PeerTagsHash uint64
}

// PayloadAggregationKey specifies the key by which a payload is aggregated.
type PayloadAggregationKey struct {
	Env         string
	Hostname    string
	Version     string
	ContainerID string
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

// stringToBytes unsafely casts a string to a byte slice.
func stringToBytes(str string) []byte {
	hdr := (*reflect.StringHeader)(unsafe.Pointer(&str))
	var slice []byte
	sliceHdr := (*reflect.SliceHeader)(unsafe.Pointer(&slice))
	sliceHdr.Data = hdr.Data
	sliceHdr.Len = hdr.Len
	sliceHdr.Cap = hdr.Len
	return slice
}

func spanKindIsClientOrProducer(spanKind string) bool {
	sk := strings.ToLower(spanKind)
	if sk == "client" || sk == "producer" {
		return true
	}
	return false
}

// NewAggregationFromSpan creates a new aggregation from the provided span and env
func NewAggregationFromSpan(s *pb.Span, origin string, aggKey PayloadAggregationKey, enablePeerSvcAgg bool, peerTagKeys []string) (Aggregation, []string) {
	synthetics := strings.HasPrefix(origin, tagSynthetics)
	agg := Aggregation{
		PayloadAggregationKey: aggKey,
		BucketsAggregationKey: BucketsAggregationKey{
			Resource:   s.Resource,
			Service:    s.Service,
			Name:       s.Name,
			SpanKind:   s.Meta[tagSpanKind],
			Type:       s.Type,
			StatusCode: getStatusCode(s),
			Synthetics: synthetics,
		},
	}
	var peerTags []string
	if spanKindIsClientOrProducer(agg.SpanKind) {
		if enablePeerSvcAgg {
			agg.PeerService = s.Meta[tagPeerService]
		}
		peerTags = getMatchingPeerTags(s, peerTagKeys)
		agg.PeerTagsHash = peerTagsHash(peerTags)
	}
	return agg, peerTags
}

func getMatchingPeerTags(s *pb.Span, peerTags []string) []string {
	if len(peerTags) == 0 {
		return nil
	}
	et := make([]string, 0, 0)
	for _, t := range peerTags {
		v, ok := s.Meta[t]
		if ok {
			et = append(et, t+":"+v)
		}
	}
	if len(et) == 0 {
		return nil
	}
	return et
}

func peerTagsHash(tags []string) uint64 {
	if len(tags) == 0 {
		return 0
	}
	h := fnv.New64a()
	for _, t := range tags {
		h.Write(stringToBytes(t))
	}
	return h.Sum64()
}

// NewAggregationFromGroup gets the Aggregation key of grouped stats.
func NewAggregationFromGroup(g *pb.ClientGroupedStats) Aggregation {
	agg := Aggregation{
		BucketsAggregationKey: BucketsAggregationKey{
			Resource:    g.Resource,
			Service:     g.Service,
			PeerService: g.PeerService,
			Name:        g.Name,
			SpanKind:    g.SpanKind,
			StatusCode:  g.HTTPStatusCode,
			Synthetics:  g.Synthetics,
		},
	}

	if g.PeerTags != nil {
		sort.Strings(g.PeerTags)
		agg.PeerTagsHash = peerTagsHash(g.PeerTags)
	}
	return agg
}
