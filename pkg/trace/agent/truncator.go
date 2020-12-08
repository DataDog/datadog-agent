// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MaxResourceLen the maximum length the resource can have
var MaxResourceLen = 5000

func init() {
	if config.HasFeature("big_resource") {
		MaxResourceLen = 15000
	}
}

const (
	// MaxMetaKeyLen the maximum length of metadata key
	MaxMetaKeyLen = 200
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
		s.Resource = traceutil.TruncateUTF8(s.Resource, MaxResourceLen)
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
			k = traceutil.TruncateUTF8(k, MaxMetaKeyLen) + "..."
			modified = true
		}

		if len(v) > MaxMetaValLen {
			v = traceutil.TruncateUTF8(v, MaxMetaValLen) + "..."
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
			k = traceutil.TruncateUTF8(k, MaxMetricsKeyLen) + "..."

			s.Metrics[k] = v
		}
	}
}
