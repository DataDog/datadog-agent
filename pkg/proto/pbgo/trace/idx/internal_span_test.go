// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInternalSpan_SetService_RemovesOldStringFromTable(t *testing.T) {
	strings := NewStringTable()
	span := &InternalSpan{
		Strings: strings,
		span: &Span{
			ServiceRef: strings.Add("old-service"),
		},
	}
	assert.Equal(t, "old-service", strings.Get(span.span.ServiceRef))
	assert.Equal(t, uint32(1), strings.refs[span.span.ServiceRef]) // 1 from Add

	span.SetService("new-service")

	assert.Equal(t, "new-service", span.Service())
	assert.Equal(t, uint32(0), strings.Lookup("old-service"))
	for _, str := range strings.strings {
		// Assert the old service is no longer in the string table
		assert.NotEqual(t, "old-service", str)
	}
}

func TestInternalSpan_SetStringAttribute_RemovesOldStringFromTable(t *testing.T) {
	strings := NewStringTable()
	span := &InternalSpan{
		Strings: strings,
		span: &Span{
			Attributes: make(map[uint32]*AnyValue),
		},
	}

	span.SetStringAttribute("old-key", "old-value")
	value, found := span.GetAttributeAsString("old-key")
	assert.True(t, found)
	assert.Equal(t, "old-value", value)

	span.SetStringAttribute("old-key", "new-value")

	value, found = span.GetAttributeAsString("old-key")
	assert.True(t, found)
	assert.Equal(t, "new-value", value)
	assert.Equal(t, uint32(0), strings.Lookup("old-value"))
	for _, str := range strings.strings {
		// Assert the old value is no longer in the string table
		assert.NotEqual(t, "old-value", str)
	}
}

func TestInternalSpan_MultipleRefsKept(t *testing.T) {
	strings := NewStringTable()
	span := &InternalSpan{
		Strings: strings,
		span: &Span{
			Attributes: make(map[uint32]*AnyValue),
		},
	}

	span.SetStringAttribute("key1", "old-value")
	span.SetStringAttribute("key2", "old-value")
	span.SetStringAttribute("key1", "new-value")

	value, found := span.GetAttributeAsString("key1")
	assert.True(t, found)
	assert.Equal(t, "new-value", value)
	oldValIdx := strings.Lookup("old-value")
	assert.NotZero(t, oldValIdx)
	assert.Equal(t, uint32(1), strings.refs[oldValIdx])
	value, found = span.GetAttributeAsString("key2")
	assert.True(t, found)
	assert.Equal(t, "old-value", value)
}
