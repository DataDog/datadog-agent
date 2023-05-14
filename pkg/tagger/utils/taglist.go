// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// TagList allows collector to incremental build a tag list
// then export it easily to []string format
type TagList struct {
	lowCardTags          map[string]bool
	orchestratorCardTags map[string]bool
	highCardTags         map[string]bool
	standardTags         map[string]bool
	splitList            map[string]string
}

// NewTagList creates a new object ready to use
func NewTagList() *TagList {
	return &TagList{
		lowCardTags:          make(map[string]bool),
		orchestratorCardTags: make(map[string]bool),
		highCardTags:         make(map[string]bool),
		standardTags:         make(map[string]bool),
		splitList:            config.Datadog.GetStringMapString("tag_value_split_separator"),
	}
}

func addTags(target map[string]bool, name string, value string, splits map[string]string) {
	if name == "" || value == "" {
		return
	}
	sep, ok := splits[name]
	if !ok {
		target[fmt.Sprintf("%s:%s", name, value)] = true
		return
	}

	for _, elt := range strings.Split(value, sep) {
		target[fmt.Sprintf("%s:%s", name, elt)] = true
	}
}

// AddHigh adds a new high cardinality tag to the map, or replace if already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddHigh(name string, value string) {
	addTags(l.highCardTags, name, value, l.splitList)
}

// AddOrchestrator adds a new orchestrator-level cardinality tag to the map, or replice if already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddOrchestrator(name string, value string) {
	addTags(l.orchestratorCardTags, name, value, l.splitList)
}

// AddLow adds a new low cardinality tag to the list, or replace if already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddLow(name string, value string) {
	addTags(l.lowCardTags, name, value, l.splitList)
}

// AddStandard adds a new standard tag to the list, or replace if already exists.
// It adds the standard tag to the low cardinality tag list as well.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddStandard(name string, value string) {
	l.AddLow(name, value)
	addTags(l.standardTags, name, value, l.splitList)
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

// Compute returns four string arrays in the format "tag:value"
// - low cardinality
// - orchestrator cardinality
// - high cardinality
// - standard tags
func (l *TagList) Compute() ([]string, []string, []string, []string) {
	return toSlice(l.lowCardTags), toSlice(l.orchestratorCardTags), toSlice(l.highCardTags), toSlice(l.standardTags)
}

func toSlice(m map[string]bool) []string {
	s := make([]string, len(m))
	index := 0
	for tag := range m {
		s[index] = tag
		index++
	}
	return s
}

// Copy creates a deep copy of the taglist object for reuse
func (l *TagList) Copy() *TagList {
	return &TagList{
		lowCardTags:          deepCopyMap(l.lowCardTags),
		orchestratorCardTags: deepCopyMap(l.orchestratorCardTags),
		highCardTags:         deepCopyMap(l.highCardTags),
		standardTags:         deepCopyMap(l.standardTags),
		splitList:            l.splitList, // constant, can be shared
	}
}

func deepCopyMap(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
