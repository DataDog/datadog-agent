// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strings"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

// Truncate checks that the span resource, meta and metrics are within the max length
// and modifies them if they are not
func (a *Agent) Truncate(s *pb.Span) {
	r, ok := a.TruncateResource(s.Resource)
	if !ok {
		log.Debugf("span.truncate: truncated `Resource` (max %d chars): %s", a.conf.MaxResourceLen, s.Resource)
	}
	s.Resource = r

	// Error - Nothing to do
	// Optional data, Meta & Metrics can be nil
	// Soft fail on those
	for k, v := range s.Meta {
		modified := false

		// Do not truncate structured meta tags.
		if isStructuredMetaKey(k) {
			continue
		}

		if len(k) > MaxMetaKeyLen {
			log.Debugf("span.truncate: truncating `Meta` key (max %d chars): %s", MaxMetaKeyLen, k)
			delete(s.Meta, k)
			k = normalize.TruncateUTF8(k, MaxMetaKeyLen) + "..."
			modified = true
		}

		if len(v) > MaxMetaValLen {
			v = normalize.TruncateUTF8(v, MaxMetaValLen) + "..."
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
			k = normalize.TruncateUTF8(k, MaxMetricsKeyLen) + "..."

			s.Metrics[k] = v
		}
	}
}

// TruncateV1 checks that the span resource, meta and metrics are within the max length
// and modifies them if they are not
func (a *Agent) TruncateV1(s *idx.InternalSpan) {
	r, ok := a.TruncateResource(s.Resource())
	if !ok {
		log.Debugf("span.truncate: truncated `Resource` (max %d chars): %s", a.conf.MaxResourceLen, s.Resource())
		s.SetResource(r)
	}

	s.MapStringAttributes(func(k, v string) (string, string, bool) {
		modified := false
		newK := k
		newV := v

		// Do not truncate structured meta tags.
		if isStructuredMetaKey(k) {
			return newK, newV, false
		}

		if len(k) > MaxMetaKeyLen {
			log.Debugf("span.truncate: truncating `Meta` key (max %d chars): %s", MaxMetaKeyLen, k)
			newK = normalize.TruncateUTF8(k, MaxMetaKeyLen) + "..."
			modified = true
		}

		if len(v) > MaxMetaValLen {
			newV = normalize.TruncateUTF8(v, MaxMetaValLen) + "..."
			modified = true
		}

		return newK, newV, modified
	})

	// Legacy spans hold these in Meta, so truncate them like Meta values. The env set by the
	// tracer is already limited by normalizeV1, but global tags or span modifiers may set it later.
	if e := s.Env(); len(e) > MaxMetaValLen {
		s.SetEnv(normalize.TruncateUTF8(e, MaxMetaValLen) + "...")
	}
	if c := s.Component(); len(c) > MaxMetaValLen {
		s.SetComponent(normalize.TruncateUTF8(c, MaxMetaValLen) + "...")
	}
	if v := s.Version(); len(v) > MaxMetaValLen {
		s.SetVersion(normalize.TruncateUTF8(v, MaxMetaValLen) + "...")
	}

	truncateNonStringAttributeKeysV1(s)
}

// truncateNonStringAttributeKeysV1 truncates the keys of the span's non-string attributes,
// as Truncate does for the keys of legacy Metrics. String attributes (legacy Meta) are handled
// by MapStringAttributes. Bytes attributes are skipped: they hold legacy meta_struct entries,
// whose keys Truncate leaves alone.
func truncateNonStringAttributeKeysV1(s *idx.InternalSpan) {
	attrs := s.Attributes()
	var longKeys []uint32
	for k, v := range attrs {
		switch v.GetValue().(type) {
		case nil, *idx.AnyValue_StringValueRef, *idx.AnyValue_BytesValue:
			continue
		}
		if len(s.Strings.Get(k)) > MaxMetricsKeyLen {
			longKeys = append(longKeys, k)
		}
	}
	// Rekey after iterating so that newly added keys are not revisited. Unlike legacy, where Meta
	// and Metrics are separate maps, a truncated key that collides with an existing attribute
	// replaces it; this needs two keys over the limit sharing a prefix, so it is accepted.
	for _, k := range longKeys {
		key := s.Strings.Get(k)
		log.Debugf("span.truncate: truncating `Metrics` key (max %d chars): %s", MaxMetricsKeyLen, key)
		v := attrs[k]
		delete(attrs, k)
		attrs[s.Strings.Add(normalize.TruncateUTF8(key, MaxMetricsKeyLen)+"...")] = v
	}
}

const (
	// MaxMetaKeyLen the maximum length of metadata key
	MaxMetaKeyLen = 200
	// MaxMetaValLen the maximum length of metadata value
	MaxMetaValLen = 25_000
	// MaxMetricsKeyLen the maximum length of a metric name key
	MaxMetricsKeyLen = MaxMetaKeyLen
)

// isStructuredMetaKey returns true when the given key is a structured meta tag.
// Structured meta tags are prefixed by `_dd.` and suffixed by .json, .msgpack
// or .protobuf. Examples include _dd.appsec.json and _dd.iast.json.
// The suffix also serves describing the serialization format in use.
func isStructuredMetaKey(key string) bool {
	return strings.HasPrefix(key, "_dd.") &&
		(strings.HasSuffix(key, ".json") ||
			strings.HasSuffix(key, ".msgpack") ||
			strings.HasSuffix(key, ".protobuf"))
}

// TruncateResource truncates a span's resource to the maximum allowed length.
// It returns true if the input was below the max size.
func (a *Agent) TruncateResource(r string) (string, bool) {
	return normalize.TruncateUTF8(r, a.conf.MaxResourceLen), len(r) <= a.conf.MaxResourceLen
}
