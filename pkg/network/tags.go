// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

// TagsSet a set of tags
type TagsSet struct {
	set          map[string]uint32
	nextTagValue uint32
}

// NewTagsSet create a new set of Tags
func NewTagsSet() *TagsSet {
	return &TagsSet{
		set:          make(map[string]uint32),
		nextTagValue: uint32(0),
	}
}

// Size return the numbers of unique tag
func (ts *TagsSet) Size() int {
	return len(ts.set)
}

// Add a tag to the set and return his index
func (ts *TagsSet) Add(tag string) (v uint32) {
	if v, found := ts.set[tag]; found {
		return v
	}
	v = ts.nextTagValue
	ts.set[tag] = v
	ts.nextTagValue++
	return v
}

// GetStrings return in order the tags
func (ts *TagsSet) GetStrings() []string {
	strs := make([]string, len(ts.set))
	for k, v := range ts.set {
		strs[v] = k
	}
	return strs
}
