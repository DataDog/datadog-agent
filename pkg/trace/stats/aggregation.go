package stats

import (
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tagHostname   = "_dd.hostname"
	tagStatusCode = "http.status_code"
	tagVersion    = "version"
	tagOrigin     = "_dd.origin"
)

// Aggregation contains all the dimension on which we aggregate statistics
// when adding or removing fields to Aggregation the methods ToTagSet, KeyLen and
// WriteKey should always be updated accordingly
type Aggregation struct {
	Env        string
	Resource   string
	Service    string
	Type       string
	Hostname   string
	StatusCode uint32
	Version    string
	Synthetics bool
}

func getStatusCode(s *pb.Span) uint32 {
	if s.Meta[tagStatusCode] == "" {
		return 0
	}
	c, err := strconv.Atoi(s.Meta[tagStatusCode])
	if err != nil {
		log.Errorf("Invalid status code %s. Using 0.", c)
		return 0
	}
	return uint32(c)
}

// NewAggregationFromSpan creates a new aggregation from the provided span and env
func NewAggregationFromSpan(s *pb.Span, env string, defaultHostname string) Aggregation {
	synthetics := strings.HasPrefix(s.Meta[tagOrigin], "synthetics")
	hostname := s.Meta[tagHostname]
	if hostname == "" {
		hostname = defaultHostname
	}
	return Aggregation{
		Env:        env,
		Resource:   s.Resource,
		Service:    s.Service,
		Type:       s.Type,
		Hostname:   hostname,
		StatusCode: getStatusCode(s),
		Version:    s.Meta[tagVersion],
		Synthetics: synthetics,
	}
}
