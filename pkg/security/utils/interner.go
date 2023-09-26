// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/DataDog/datadog-agent/pkg/util/intern"
)

// StringInterner is a best-effort LRU-based string deduplicator
type StringInterner struct {
	interner *intern.StringInterner
}

// NewStringInterner returns a new LRUStringInterner, with the cache size provided
// if the cache size is negative this function will panic
func NewStringInterner() *StringInterner {
	return &StringInterner{
		interner: intern.NewStringInterner(),
	}
}

// Deduplicate returns a possibly de-duplicated string
func (si *StringInterner) Deduplicate(value string) string {
	return si.deduplicateUnsafe(value)
}

func (si *StringInterner) deduplicateUnsafe(value string) string {
	return si.interner.GetString(value).Get()
}

// DeduplicateSlice returns a possibly de-duplicated string slice
func (si *StringInterner) DeduplicateSlice(values []string) {
	for i := range values {
		values[i] = si.deduplicateUnsafe(values[i])
	}
}
