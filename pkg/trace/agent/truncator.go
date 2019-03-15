package agent

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// MaxResourceLen the maximum length the resource can have
	MaxResourceLen = 5000
	// MaxMetaKeyLen the maximum length of metadata key
	MaxMetaKeyLen = 100
	// MaxMetaValLen the maximum length of metadata value
	MaxMetaValLen = 5000
	// MaxMetricsKeyLen the maximum length of a metric name key
	MaxMetricsKeyLen = MaxMetaKeyLen
)

// Truncate checks that the span resource, meta and metrics are within the max length
// and modifies them if they are not
func Truncate(s *pb.Span) {
	// Resource
	if len(s.Resource) > MaxResourceLen {
		s.Resource = truncateString(s.Resource, MaxResourceLen)
		log.Debugf("span.truncate: truncated `Resource` (max %d chars): %s", MaxResourceLen, s.Resource)
	}

	// Error - Nothing to do
	// Optional data, Meta & Metrics can be nil
	// Soft fail on those
	for k, v := range s.Meta {
		modified := false

		if len(k) > MaxMetaKeyLen {
			log.Debugf("span.truncate: truncating `Meta` key (max %d chars): %s", MaxMetaKeyLen, k)
			delete(s.Meta, k)
			k = truncateString(k, MaxMetaKeyLen) + "..."
			modified = true
		}

		if len(v) > MaxMetaValLen {
			v = truncateString(v, MaxMetaValLen) + "..."
			modified = true
		}

		if modified {
			s.Meta[k] = v
		}
	}

	for k, v := range s.Metrics {
		if len(k) > MaxMetricsKeyLen {
			log.Debugf("span.truncate: truncating `Metrics` key (max %d chars): %s", MaxMetricsKeyLen, k)
			delete(s.Metrics, k)
			k = truncateString(k, MaxMetricsKeyLen) + "..."

			s.Metrics[k] = v
		}
	}
}

// truncateString truncates the given string to make sure it uses less than limit bytes.
// If the last character is an utf8 character that would be splitten, it removes it
// entirely to make sure the resulting string is not broken.
func truncateString(s string, limit int) string {
	var lastValidIndex int
	for i := range s {
		if i > limit {
			return s[:lastValidIndex]
		}
		lastValidIndex = i
	}
	return s
}
