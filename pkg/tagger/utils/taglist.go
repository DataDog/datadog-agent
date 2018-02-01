// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package utils

import (
	"fmt"
	"strings"
)

// TagList allows collector to incremental build a tag list
// then export it easily to []string format
type TagList struct {
	lowCardTags  map[string]bool
	highCardTags map[string]bool
}

// NewTagList creates a new object ready to use
func NewTagList() *TagList {
	return &TagList{
		lowCardTags:  make(map[string]bool),
		highCardTags: make(map[string]bool),
	}
}

// AddHigh adds a new high cardinality tag to the map, or replace if already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddHigh(name string, value string) {
	if name != "" && value != "" {
		l.highCardTags[fmt.Sprintf("%s:%s", name, value)] = true
	}
}

// AddLow adds a new low cardinality tag to the list, or replace if already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddLow(name string, value string) {
	if name != "" && value != "" {
		l.lowCardTags[fmt.Sprintf("%s:%s", name, value)] = true
	}
}

// AddAuto determine the tag cardinality and will call the proper method AddLow or AddHigh
// if the name value starts with '+' character
func (l *TagList) AddAuto(name, value string) {
	if strings.HasPrefix(name, "+") {
		l.AddHigh(name[1:], value)
		return
	}
	l.AddLow(name, value)
}

// Compute returns two string arrays in the format "tag:value"
// first array is low cardinality tags, second is high card ones
func (l *TagList) Compute() ([]string, []string) {
	low := make([]string, len(l.lowCardTags))
	high := make([]string, len(l.highCardTags))

	index := 0
	for tag := range l.lowCardTags {
		low[index] = tag
		index++
	}
	index = 0
	for tag := range l.highCardTags {
		high[index] = tag
		index++
	}
	return low, high
}
