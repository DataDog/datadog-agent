// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package stats contains the logic to process APM stats.
package stats

import (
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	"google.golang.org/genproto/googleapis/rpc/code"
)

var ddRegistry = semantics.DefaultRegistry()

const (
	tagSynthetics    = "synthetics"
	tagServiceSource = "_dd.svc_src"
)

// Aggregation contains all the dimension on which we aggregate statistics.
type Aggregation struct {
	BucketsAggregationKey
	PayloadAggregationKey
}

// BucketsAggregationKey specifies the key by which a bucket is aggregated.
type BucketsAggregationKey struct {
	Service                    string
	Name                       string
	Resource                   string
	Type                       string
	SpanKind                   string
	StatusCode                 uint32
	Synthetics                 bool
	PeerTagsHash               uint64
	SpanDerivedPrimaryTagsHash uint64
	AdditionalMetricTagsHash   uint64
	ServiceSource              string
	IsTraceRoot                pb.Trilean
	GRPCStatusCode             string
	HTTPMethod                 string
	HTTPEndpoint               string
}

// PayloadAggregationKey specifies the key by which a payload is aggregated.
type PayloadAggregationKey struct {
	Env             string
	Hostname        string
	Version         string
	ContainerID     string
	GitCommitSha    string
	ImageTag        string
	Lang            string
	ProcessTagsHash uint64
	BaseService     string
}

func toStatusCode(v int64) (uint32, bool) {
	if v < 0 || v > math.MaxUint32 {
		return 0, false
	}
	return uint32(v), true
}

func getStatusCode(meta map[string]string, metrics map[string]float64) uint32 {
	a := semantics.NewDDSpanAccessor(meta, metrics)
	v, ok := semantics.LookupInt64(ddRegistry, a, semantics.ConceptHTTPStatusCode)
	if !ok {
		return 0
	}
	code, ok := toStatusCode(v)
	if !ok {
		return 0
	}
	return code
}

func getStatusCodeV1(s *idx.InternalSpan) uint32 {
	a := semantics.NewDDSpanAccessorV1(s)
	v, ok := semantics.LookupInt64(ddRegistry, a, semantics.ConceptHTTPStatusCode)
	if !ok {
		return 0
	}
	code, ok := toStatusCode(v)
	if !ok {
		return 0
	}
	return code
}

// NewAggregationFromSpan creates a new aggregation from the provided span and env
func NewAggregationFromSpan(s *StatSpan, origin string, aggKey PayloadAggregationKey) Aggregation {
	synthetics := strings.HasPrefix(origin, tagSynthetics)
	var isTraceRoot pb.Trilean
	if s.parentID == 0 {
		isTraceRoot = pb.Trilean_TRUE
	} else {
		isTraceRoot = pb.Trilean_FALSE
	}
	agg := Aggregation{
		PayloadAggregationKey: aggKey,
		BucketsAggregationKey: BucketsAggregationKey{
			Resource:                   s.resource,
			Service:                    s.service,
			Name:                       s.name,
			SpanKind:                   s.spanKind,
			Type:                       s.typ,
			StatusCode:                 s.statusCode,
			ServiceSource:              s.serviceSource,
			Synthetics:                 synthetics,
			IsTraceRoot:                isTraceRoot,
			GRPCStatusCode:             s.grpcStatusCode,
			PeerTagsHash:               tagsFnvHash(s.matchingPeerTags),
			SpanDerivedPrimaryTagsHash: tagsFnvHash(s.matchingSpanDerivedPrimaryTags),
			HTTPMethod:                 s.httpMethod,
			HTTPEndpoint:               s.httpEndpoint,
		},
	}
	return agg
}

func processTagsHash(processTags string) uint64 {
	if processTags == "" {
		return 0
	}
	return tagsFnvHash(strings.Split(processTags, ","))
}

func tagsFnvHash(tags []string) uint64 {
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
			Resource:                   g.Resource,
			Service:                    g.Service,
			Name:                       g.Name,
			SpanKind:                   g.SpanKind,
			StatusCode:                 g.HTTPStatusCode,
			Synthetics:                 g.Synthetics,
			PeerTagsHash:               tagsFnvHash(g.PeerTags),
			SpanDerivedPrimaryTagsHash: tagsFnvHash(g.SpanDerivedPrimaryTags),
			AdditionalMetricTagsHash:   tagsFnvHash(g.AdditionalMetricTags),
			ServiceSource:              g.ServiceSource,
			IsTraceRoot:                g.IsTraceRoot,
			GRPCStatusCode:             g.GRPCStatusCode,
			HTTPMethod:                 g.HTTPMethod,
			HTTPEndpoint:               g.HTTPEndpoint,
		},
	}
}

/*
The gRPC codes Google API checks for "CANCELLED". Sometimes we receive "Canceled" from upstream,
sometimes "CANCELLED", which is why both spellings appear in the map.
For multi-word codes, sometimes from upstream we receive them as one word, such as DeadlineExceeded.
Google's API checks for strings with an underscore and in all caps, and would only recognize codes
formatted like "ALREADY_EXISTS" or "DEADLINE_EXCEEDED"
*/
var grpcStatusMap = map[string]string{
	"CANCELLED":          "1",
	"CANCELED":           "1",
	"INVALIDARGUMENT":    "3",
	"DEADLINEEXCEEDED":   "4",
	"NOTFOUND":           "5",
	"ALREADYEXISTS":      "6",
	"PERMISSIONDENIED":   "7",
	"RESOURCEEXHAUSTED":  "8",
	"FAILEDPRECONDITION": "9",
	"OUTOFRANGE":         "11",
	"DATALOSS":           "15",
}

// parseGRPCStatusString converts a raw gRPC status string (numeric, enum name, or
// "StatusCode."-prefixed) to its canonical numeric string form, or "" if unrecognized.
func parseGRPCStatusString(strC string) string {
	if c, err := strconv.ParseUint(strC, 10, 32); err == nil {
		return strconv.FormatUint(c, 10)
	}
	strC = strings.TrimPrefix(strC, "StatusCode.") // Some tracers send status code values prefixed by "StatusCode."
	strCUpper := strings.ToUpper(strC)
	if statusCode, exists := grpcStatusMap[strCUpper]; exists {
		return statusCode
	}
	if codeNum, found := code.Code_value[strCUpper]; found {
		return strconv.Itoa(int(codeNum))
	}
	return ""
}

func getGRPCStatusCode(meta map[string]string, metrics map[string]float64) string {
	a := semantics.NewDDSpanAccessor(meta, metrics)
	strC := semantics.LookupString(ddRegistry, a, semantics.ConceptGRPCStatusCode)
	if strC == "" {
		return ""
	}
	return parseGRPCStatusString(strC)
}

func getGRPCStatusCodeV1(s *idx.InternalSpan) string {
	a := semantics.NewDDSpanAccessorV1(s)
	strC := semantics.LookupString(ddRegistry, a, semantics.ConceptGRPCStatusCode)
	if strC == "" {
		return ""
	}
	return parseGRPCStatusString(strC)
}
