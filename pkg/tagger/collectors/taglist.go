// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package collectors

import (
	"fmt"
)

// tagList allows collector to incremently build a tag list
// then export it easily to []string format
type tagList struct {
	lowCardTags  map[string]string
	highCardTags map[string]string
}

func newTagList() *tagList {
	return &tagList{
		lowCardTags:  make(map[string]string),
		highCardTags: make(map[string]string),
	}
}

func (l *tagList) add(name string, value string, highCard bool) {
	if highCard {
		l.addHigh(name, value)
	} else {
		l.addLow(name, value)
	}
}

func (l *tagList) addHigh(name string, value string) {
	l.highCardTags[name] = value
}

func (l *tagList) addLow(name string, value string) {
	l.lowCardTags[name] = value
}

// compute returns two string arrays in the format "tag:value"
// first array is low cardinality tags, second is high card ones
func (l *tagList) compute() ([]string, []string) {
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
