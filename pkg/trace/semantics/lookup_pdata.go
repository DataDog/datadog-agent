// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"strconv"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// NewPDataMapAccessor returns an AttrGetter for a pcommon.Map. All value types are returned as string
// (Double and Int are formatted so LookupFloat64/LookupInt64 can parse them).
func NewPDataMapAccessor(attrs pcommon.Map) AttrGetter {
	return func(key string) string {
		v, ok := attrs.Get(key)
		if !ok {
			return ""
		}
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			return strconv.FormatFloat(v.Double(), 'f', -1, 64)
		case pcommon.ValueTypeInt:
			return strconv.FormatInt(v.Int(), 10)
		default:
			return v.Str()
		}
	}
}

// NewOTelSpanAccessor returns an AttrGetter for OTel span and resource attributes.
// Span attributes take precedence over resource attributes.
func NewOTelSpanAccessor(spanAttrs, resAttrs pcommon.Map) AttrGetter {
	return NewCombinedAccessor(
		NewPDataMapAccessor(spanAttrs),
		NewPDataMapAccessor(resAttrs),
	)
}
