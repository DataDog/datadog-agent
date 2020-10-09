// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"sort"
	"strings"
)

// Tag represents a key / value dimension on traces and stats.
type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// String returns a string representation of a tag
func (t Tag) String() string {
	return t.Name + ":" + t.Value
}

// SplitTag splits the tag into group and value. If it doesn't have a separator
// the empty string will be used for the group.
func SplitTag(tag string) (group, value string) {
	split := strings.SplitN(tag, ":", 2)
	if len(split) == 1 {
		return "", split[0]
	}
	return split[0], split[1]
}

// NewTagFromString returns a new Tag from a raw string
func NewTagFromString(raw string) Tag {
	name, val := SplitTag(raw)
	return Tag{name, val}
}

// TagSet is a combination of given tags, it is equivalent to contexts that we use for metrics.
// Although we choose a different terminology here to avoid confusion, and tag sets do not have
// a notion of activeness over time. A tag can be:
//   • one of the fixed ones we defined in the span structure: service, resource and host
//   • one of the arbitrary metadata key included in the span (it needs to be turned on manually)
//
// When we track statistics by tag sets, we basically track every tag combination we're interested
// in to create dimensions, for instance:
//   • (service)
//   • (service, environment)
//   • (service, host)
//   • (service, resource, environment)
//   • (service, resource)
//   • ..
type TagSet []Tag

// NewTagSetFromString returns a new TagSet from a raw string
func NewTagSetFromString(raw string) TagSet {
	var tags TagSet
	for _, t := range strings.Split(raw, ",") {
		tags = append(tags, NewTagFromString(t))
	}
	return tags
}

// TagKey returns a unique key from the string given and the tagset, useful to index stuff on tagsets
func (t TagSet) TagKey(m string) string {
	tagStrings := make([]string, len(t))
	for i, tag := range t {
		tagStrings[i] = tag.String()
	}
	sort.Strings(tagStrings)
	return m + "|" + strings.Join(tagStrings, ",")
}

func (t TagSet) Len() int      { return len(t) }
func (t TagSet) Swap(i, j int) { t[i], t[j] = t[j], t[i] }
func (t TagSet) Less(i, j int) bool {
	if t[i].Name == t[j].Name {
		return t[i].Value < t[j].Value
	}
	return t[i].Name < t[j].Name
}

// Key returns a string representing a new set of tags.
func (t TagSet) Key() string {
	s := make([]string, len(t))
	for i, t := range t {
		s[i] = t.String()
	}
	sort.Strings(s)
	return strings.Join(s, ",")
}

// Get the tag with the particular name
func (t TagSet) Get(name string) Tag {
	for _, tag := range t {
		if tag.Name == name {
			return tag
		}
	}
	return Tag{}
}

// Unset returns a new tagset without a given value
func (t TagSet) Unset(name string) TagSet {
	var j int
	var t2 TagSet
	for i, tag := range t {
		if tag.Name == name {
			j = i + 1
			break
		}
		t2 = append(t2, tag)
	}
	for i := j; i < len(t); i++ {
		t2 = append(t2, t[i])
	}
	return t2
}

// Match returns a new tag set with only the tags matching the given groups.
func (t TagSet) Match(groups []string) TagSet {
	if len(groups) == 0 {
		return nil
	}
	var match []Tag
	for _, g := range groups {
		tag := t.Get(g)
		if tag.Value == "" {
			continue
		}
		match = append(match, tag)
	}
	ts := TagSet(match)
	sort.Sort(ts)
	return ts
}

// HasExactly returns true if we have tags only for the given groups.
func (t TagSet) HasExactly(groups []string) bool {
	if len(groups) != len(t) {
		return false
	}
	// FIXME quadratic
	for _, g := range groups {
		if t.Get(g).Name == "" {
			return false
		}
	}
	return true
}

// MatchFilters returns a tag set of the tags that match certain filters.
// A filter is defined as : "KEY:VAL" where:
//  * KEY is a non-empty string
//  * VALUE is a string (can be empty)
// A tag {Name:k, Value:v} from the input tag set will match if:
//  * KEY==k and VALUE is non-empty and v==VALUE
//  * KEY==k and VALUE is empty (don't care about v)
func (t TagSet) MatchFilters(filters []string) TagSet {
	// FIXME: ugly ?
	filterMap := make(map[string]map[string]struct{})

	for _, f := range filters {
		g, v := SplitTag(f)
		_, ok := filterMap[g]
		if !ok {
			filterMap[g] = make(map[string]struct{})
		}
		if v != "" {
			filterMap[g][v] = struct{}{}
		}
	}

	matchedFilters := TagSet{}

	for _, tag := range t {
		vals, ok := filterMap[tag.Name]
		if ok {
			if len(vals) == 0 {
				matchedFilters = append(matchedFilters, tag)
			} else {
				_, ok := vals[tag.Value]
				if ok {
					matchedFilters = append(matchedFilters, tag)
				}
			}
		}
	}
	return matchedFilters
}

// MergeTagSets merge two tag sets lazily
func MergeTagSets(t1, t2 TagSet) TagSet {
	if t1 == nil {
		return t2
	}
	if t2 == nil {
		return t1
	}
	t := append(t1, t2...)

	if len(t) < 2 {
		return t
	}

	// sorting is actually expensive so skip it if we can
	if !sort.IsSorted(t) {
		sort.Sort(t)
	}

	last := t[0]
	idx := 1
	for i := 1; i < len(t); i++ {
		if t[i].Name != last.Name || t[i].Value != last.Value {
			last = t[i]
			t[idx] = last
			idx++

		}
	}
	return t[:idx]
}

// TagGroup will return the tag group from the given string. For example,
// "host:abc" => "host"
func TagGroup(tag string) string {
	for i, c := range tag {
		if c == ':' {
			return tag[0:i]
		}
	}
	return ""
}

// FilterTags will return the tags that have the given group.
func FilterTags(tags, groups []string) []string {
	var out []string
	for _, t := range tags {
		tg := TagGroup(t)
		for _, g := range groups {
			if g == tg {
				out = append(out, t)
				break
			}
		}
	}
	return out
}
