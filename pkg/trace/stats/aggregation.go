package stats

import (
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tagHostname   = "_dd.hostname"
	tagStatusCode = "http.status_code"
	tagVersion    = "version"
	tagOrigin     = "_dd.origin"
	tagSynthetics = "synthetics"
)

// Aggregation contains all the dimension on which we aggregate statistics.
type Aggregation struct {
	BucketsAggregationKey
	PayloadAggregationKey
}

// BucketsAggregationKey specifies the key by which a bucket is aggregated.
type BucketsAggregationKey struct {
	Service    string
	Name       string
	Resource   string
	Type       string
	StatusCode uint32
	Synthetics bool
}

// PayloadAggregationKey specifies the key by which a payload is aggregated.
type PayloadAggregationKey struct {
	Env         string
	Hostname    string
	Version     string
	ContainerID string
}

func getStatusCode(s *pb.Span) uint32 {
	strC := traceutil.GetMetaDefault(s, tagStatusCode, "")
	if strC == "" {
		return 0
	}
	c, err := strconv.Atoi(strC)
	if err != nil {
		log.Debugf("Invalid status code %s. Using 0.", strC)
		return 0
	}
	return uint32(c)
}

// NewAggregationFromSpan creates a new aggregation from the provided span and env
func NewAggregationFromSpan(s *pb.Span, env string, agentHostname, containerID string) Aggregation {
	synthetics := strings.HasPrefix(traceutil.GetMetaDefault(s, tagOrigin, ""), tagSynthetics)
	hostname := traceutil.GetMetaDefault(s, tagHostname, "")
	if hostname == "" {
		hostname = agentHostname
	}
	return Aggregation{
		PayloadAggregationKey: PayloadAggregationKey{
			Env:         env,
			Hostname:    hostname,
			Version:     traceutil.GetMetaDefault(s, tagVersion, ""),
			ContainerID: containerID,
		},
		BucketsAggregationKey: BucketsAggregationKey{
			Resource:   s.Resource,
			Service:    s.Service,
			Name:       s.Name,
			Type:       s.Type,
			StatusCode: getStatusCode(s),
			Synthetics: synthetics,
		},
	}
}

// NewAggregationFromGroup gets the Aggregation key of grouped stats.
func NewAggregationFromGroup(g pb.ClientGroupedStats) Aggregation {
	return Aggregation{
		BucketsAggregationKey: BucketsAggregationKey{
			Resource:   g.Resource,
			Service:    g.Service,
			Name:       g.Name,
			StatusCode: g.HTTPStatusCode,
			Synthetics: g.Synthetics,
		},
	}
}
