package stats

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"strings"
)

const (
	hostnameTag   = "_dd.hostname"
	statusCodeTag = "http.status_code"
	versionTag    = "version"
)

// When adding or removing fields to aggregation the methods toTags, keyLen and
// writeKey should always be updated accordingly
type aggregation struct {
	Env        string
	Resource   string
	Service    string
	Hostname   string
	StatusCode string
	Version    string
}

func newAggregationFromSpan(s *pb.Span, env string) aggregation {
	return aggregation{
		Env:        env,
		Resource:   s.Resource,
		Service:    s.Service,
		Hostname:   s.Meta[hostnameTag],
		StatusCode: s.Meta[statusCodeTag],
		Version:    s.Meta[versionTag],
	}
}

func (aggr *aggregation) toTags() TagSet {
	tagSet := make(TagSet, 3, 7)
	tagSet[0] = Tag{"env", aggr.Env}
	tagSet[1] = Tag{"resource", aggr.Resource}
	tagSet[2] = Tag{"service", aggr.Service}
	if len(aggr.Hostname) > 0 {
		tagSet = append(tagSet, Tag{hostnameTag, aggr.Hostname})
	}
	if len(aggr.StatusCode) > 0 {
		tagSet = append(tagSet, Tag{statusCodeTag, aggr.StatusCode})
	}
	if len(aggr.Version) > 0 {
		tagSet = append(tagSet, Tag{versionTag, aggr.Version})
	}
	return tagSet
}

func (aggr *aggregation) keyLen() int {
	length := len("env:") + len(aggr.Env) + len(",resource:") + len(aggr.Resource) + len(",service:") + len(aggr.Service)
	if len(aggr.Hostname) > 0 {
		// +2 for "," and ":" separator
		length += 1 + len(hostnameTag) + 1 + len(aggr.Hostname)
	}
	if len(aggr.StatusCode) > 0 {
		// +2 for "," and ":" separator
		length += 1 + len(statusCodeTag) + 1 + len(aggr.StatusCode)
	}
	if len(aggr.Version) > 0 {
		// +2 for "," and ":" separator
		length += 1 + len(versionTag) + 1 + len(aggr.Version)
	}
	return length
}

func (aggr *aggregation) writeKey(b *strings.Builder) {
	b.WriteString("env:")
	b.WriteString(aggr.Env)
	b.WriteString(",resource:")
	b.WriteString(aggr.Resource)
	b.WriteString(",service:")
	b.WriteString(aggr.Service)

	// Keys should be written in lexicographical order of the tag name
	if len(aggr.Hostname) > 0 {
		b.WriteRune(',')
		b.WriteString(hostnameTag)
		b.WriteRune(':')
		b.WriteString(aggr.Hostname)
	}
	if len(aggr.StatusCode) > 0 {
		b.WriteRune(',')
		b.WriteString(statusCodeTag)
		b.WriteRune(':')
		b.WriteString(aggr.StatusCode)
	}
	if len(aggr.Version) > 0 {
		b.WriteRune(',')
		b.WriteString(versionTag)
		b.WriteRune(':')
		b.WriteString(aggr.Version)
	}
}
