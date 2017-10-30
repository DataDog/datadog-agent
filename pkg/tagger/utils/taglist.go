// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package utils

import (
	"fmt"
)

// TagList allows collector to incremental build a tag list
// then export it easily to []string format
type TagList struct {
	lowCardTags  map[string]string
	highCardTags map[string]string
}

// NewTagList creates a new object ready to use
func NewTagList() *TagList {
	return &TagList{
		lowCardTags:  make(map[string]string),
		highCardTags: make(map[string]string),
	}
}

// AddHigh adds a new high cardinality tag to the list, or replace if name already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddHigh(name string, value string) {
	if len(name) > 0 && len(value) > 0 {
		l.highCardTags[name] = value
	}
}

// AddLow adds a new low cardinality tag to the list, or replace if name already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddLow(name string, value string) {
	if len(name) > 0 && len(value) > 0 {
		l.lowCardTags[name] = value
	}
}

// Compute returns two string arrays in the format "tag:value"
// first array is low cardinality tags, second is high card ones
func (l *TagList) Compute() ([]string, []string) {
	low := make([]string, len(l.lowCardTags))
	high := make([]string, len(l.highCardTags))

	index := 0
	for k, v := range l.lowCardTags {
		low[index] = fmt.Sprintf("%s:%s", k, v)
		index++
	}
	index = 0
	for k, v := range l.highCardTags {
		high[index] = fmt.Sprintf("%s:%s", k, v)
		index++
	}
	return low, high
}
