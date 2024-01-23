// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

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
	panic("not called")
}

func addTags(target map[string]bool, name string, value string, splits map[string]string) {
	panic("not called")
}

// AddHigh adds a new high cardinality tag to the map, or replace if already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddHigh(name string, value string) {
	panic("not called")
}

// AddOrchestrator adds a new orchestrator-level cardinality tag to the map, or replice if already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddOrchestrator(name string, value string) {
	panic("not called")
}

// AddLow adds a new low cardinality tag to the list, or replace if already exists.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddLow(name string, value string) {
	panic("not called")
}

// AddStandard adds a new standard tag to the list, or replace if already exists.
// It adds the standard tag to the low cardinality tag list as well.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddStandard(name string, value string) {
	panic("not called")
}

// AddAuto determine the tag cardinality and will call the proper method AddLow or AddHigh
// if the name value starts with '+' character
func (l *TagList) AddAuto(name, value string) {
	panic("not called")
}

// Compute returns four string arrays in the format "tag:value"
// - low cardinality
// - orchestrator cardinality
// - high cardinality
// - standard tags
func (l *TagList) Compute() ([]string, []string, []string, []string) {
	panic("not called")
}

func toSlice(m map[string]bool) []string {
	panic("not called")
}

// Copy creates a deep copy of the taglist object for reuse
func (l *TagList) Copy() *TagList {
	panic("not called")
}

func deepCopyMap(in map[string]bool) map[string]bool {
	panic("not called")
}
