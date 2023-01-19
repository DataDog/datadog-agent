// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func fillAttributeMap(attrs pcommon.Map, mp map[string]string) {
	attrs.Clear()
	attrs.EnsureCapacity(len(mp))
	for k, v := range mp {
		attrs.PutStr(k, v)
	}
}

// NewAttributeMap creates a new attribute map (string only)
// from a Go map
func NewAttributeMap(mp map[string]string) pcommon.Map {
	attrs := pcommon.NewMap()
	fillAttributeMap(attrs, mp)
	return attrs
}
