package agent

import (
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	log "github.com/cihub/seelog"
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
		s.Resource = s.Resource[:MaxResourceLen]
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
			k = k[:MaxMetaKeyLen] + "..."
			modified = true
		}

		if len(v) > MaxMetaValLen {
			v = v[:MaxMetaValLen] + "..."
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
			k = k[:MaxMetricsKeyLen] + "..."

			s.Metrics[k] = v
		}
	}
}
