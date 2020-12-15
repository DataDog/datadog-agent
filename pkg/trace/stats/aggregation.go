package stats

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

const (
	tagHostname   = "_dd.hostname"
	tagStatusCode = "http.status_code"
	tagVersion    = "version"
	tagOrigin     = "_dd.origin"
	tagSynthetics = "synthetics"
)

// Aggregation contains all the dimension on which we aggregate statistics
// when adding or removing fields to Aggregation the methods ToTagSet, KeyLen and
// WriteKey should always be updated accordingly
type Aggregation struct {
	Env        string
	Resource   string
	Service    string
	Hostname   string
	StatusCode string
	Version    string
	Synthetics bool
}

// NewAggregationFromSpan creates a new aggregation from the provided span and env
func NewAggregationFromSpan(s *pb.Span, env string) Aggregation {
	synthetics := strings.HasPrefix(s.Meta[tagOrigin], "synthetics")

	return Aggregation{
		Env:        env,
		Resource:   s.Resource,
		Service:    s.Service,
		Hostname:   s.Meta[tagHostname],
		StatusCode: s.Meta[tagStatusCode],
		Version:    s.Meta[tagVersion],
		Synthetics: synthetics,
	}
}

// NewAggregation creates a new aggregation from the provided fields
func NewAggregation(env string, resource string, service string, hostname string, statusCode string, version string, synthetics bool) Aggregation {
	return Aggregation{
		Env:        env,
		Resource:   resource,
		Service:    service,
		Hostname:   hostname,
		StatusCode: statusCode,
		Version:    version,
		Synthetics: synthetics,
	}
}

// ToTagSet creates a TagSet with the fields of the aggregation
func (aggr *Aggregation) ToTagSet() TagSet {
	tagSet := make(TagSet, 3, 7)
	tagSet[0] = Tag{"env", aggr.Env}
	tagSet[1] = Tag{"resource", aggr.Resource}
	tagSet[2] = Tag{"service", aggr.Service}
	if len(aggr.Hostname) > 0 {
		tagSet = append(tagSet, Tag{tagHostname, aggr.Hostname})
	}
	if len(aggr.StatusCode) > 0 {
		tagSet = append(tagSet, Tag{tagStatusCode, aggr.StatusCode})
	}
	if len(aggr.Version) > 0 {
		tagSet = append(tagSet, Tag{tagVersion, aggr.Version})
	}
	if aggr.Synthetics {
		tagSet = append(tagSet, Tag{tagSynthetics, "true"})
	}
	return tagSet
}

// KeyLen computes the length of the string required to generate the string representing this aggregation
func (aggr *Aggregation) KeyLen() int {
	length := len("env:") + len(aggr.Env) + len(",resource:") + len(aggr.Resource) + len(",service:") + len(aggr.Service)
	if len(aggr.Hostname) > 0 {
		// +2 for "," and ":" separator
		length += 1 + len(tagHostname) + 1 + len(aggr.Hostname)
	}
	if len(aggr.StatusCode) > 0 {
		// +2 for "," and ":" separator
		length += 1 + len(tagStatusCode) + 1 + len(aggr.StatusCode)
	}
	if len(aggr.Version) > 0 {
		// +2 for "," and ":" separator
		length += 1 + len(tagVersion) + 1 + len(aggr.Version)
	}
	if aggr.Synthetics {
		// +2 for "," and ":" separator
		length += 1 + len(tagSynthetics) + 1 + len("true")
	}
	return length
}

// WriteKey writes the aggregation to the provided strings.Builder in its canonical form
func (aggr *Aggregation) WriteKey(b *strings.Builder) {
	b.WriteString("env:")
	b.WriteString(aggr.Env)
	b.WriteString(",resource:")
	b.WriteString(aggr.Resource)
	b.WriteString(",service:")
	b.WriteString(aggr.Service)

	// Keys should be written in lexicographical order of the tag name
	if len(aggr.Hostname) > 0 {
		b.WriteString("," + tagHostname + ":")
		b.WriteString(aggr.Hostname)
	}
	if len(aggr.StatusCode) > 0 {
		b.WriteString("," + tagStatusCode + ":")
		b.WriteString(aggr.StatusCode)
	}
	if len(aggr.Version) > 0 {
		b.WriteString("," + tagVersion + ":")
		b.WriteString(aggr.Version)
	}
	if aggr.Synthetics {
		b.WriteString("," + tagSynthetics + ":")
		b.WriteString("true")
	}
}
