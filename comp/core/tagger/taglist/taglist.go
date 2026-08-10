// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package taglist provides helpers to interact with a tag list.
package taglist

import (
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"unique"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// denylistSetting lists the tag names that must never be built out of
// workload-controlled metadata.
const denylistSetting = "workload_tags_denylist"

// TagList allows collector to incremental build a tag list
// then export it easily to []string format
type TagList struct {
	lowCardTags          map[string]bool
	orchestratorCardTags map[string]bool
	highCardTags         map[string]bool
	standardTags         map[string]bool
	splitList            map[string]string
	// denylist is a read-only set of tag names refused from workload-controlled
	// metadata, shared between tag lists. Only the Add*FromWorkload methods
	// honor it: the tags the Agent computes itself are never filtered out.
	denylist map[string]struct{}
}

// NewTagList creates a new object ready to use
func NewTagList() *TagList {
	return &TagList{
		lowCardTags:          make(map[string]bool),
		orchestratorCardTags: make(map[string]bool),
		highCardTags:         make(map[string]bool),
		standardTags:         make(map[string]bool),
		splitList:            pkgconfigsetup.Datadog().GetStringMapString("tag_value_split_separator"),
		denylist:             deniedTagNames(),
	}
}

// deniedTagNamesCache memoizes the parsed denylist: NewTagList runs for every
// workloadmeta event, so the set is only rebuilt when the setting changes.
var deniedTagNamesCache atomic.Pointer[denylistCache]

type denylistCache struct {
	raw []string
	set map[string]struct{}
}

// deniedTagNames returns the configured denylist as a set, for O(1) lookups.
// The returned map is shared and must not be modified.
func deniedTagNames() map[string]struct{} {
	raw := pkgconfigsetup.Datadog().GetStringSlice(denylistSetting)

	if cached := deniedTagNamesCache.Load(); cached != nil && slices.Equal(cached.raw, raw) {
		return cached.set
	}

	set := make(map[string]struct{}, len(raw))
	for _, name := range raw {
		if name := normalizeTagName(name); name != "" {
			set[name] = struct{}{}
		}
	}

	deniedTagNamesCache.Store(&denylistCache{raw: raw, set: set})

	return set
}

// normalizeTagName reduces a tag name to what consumers actually read as the
// tag name, so that the denylist matches whichever form the workload used: the
// '+' high cardinality prefix is stripped, the name is lowercased, and anything
// from the first ':' on is dropped, since tags are serialized as "name:value"
// and split on their first ':'.
func normalizeTagName(name string) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "+")
	name, _, _ = strings.Cut(name, ":")
	return strings.ToLower(strings.TrimSpace(name))
}

// IsDenied reports whether name is refused from workload-controlled metadata.
func (l *TagList) IsDenied(name string) bool {
	if len(l.denylist) == 0 {
		return false
	}

	_, denied := l.denylist[normalizeTagName(name)]
	return denied
}

func addTags(target map[string]bool, name string, value string, splits map[string]string) {
	if name == "" || value == "" {
		return
	}
	sep, ok := splits[name]
	if !ok {
		// Some tags may have the same values across different entities. This is
		// common for tags like "kube_namespace", "pod_phase", "env",
		// "kube_qos", etc. Using the unique package helps optimize memory usage
		// in such cases.
		key := unique.Make(name + ":" + value)
		target[key.Value()] = true
		return
	}

	for elt := range strings.SplitSeq(value, sep) {
		key := unique.Make(name + ":" + elt)
		target[key.Value()] = true
	}
}

// AddHigh adds a new high cardinality tag to the map.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddHigh(name string, value string) {
	addTags(l.highCardTags, name, value, l.splitList)
}

// AddOrchestrator adds a new orchestrator-level cardinality tag to the map.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddOrchestrator(name string, value string) {
	addTags(l.orchestratorCardTags, name, value, l.splitList)
}

// AddLow adds a new low cardinality tag to the map.
// It will skip empty values/names, so it's safe to use without verifying the value is not empty.
func (l *TagList) AddLow(name string, value string) {
	addTags(l.lowCardTags, name, value, l.splitList)
}

// AddStandard adds a new standard tag to the map.
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

// AddLowFromWorkload behaves like AddLow for a tag name coming from
// workload-controlled metadata, dropping the denied ones.
func (l *TagList) AddLowFromWorkload(name string, value string) {
	if l.denied(name) {
		return
	}
	l.AddLow(name, value)
}

// AddHighFromWorkload behaves like AddHigh for a tag name coming from
// workload-controlled metadata, dropping the denied ones.
func (l *TagList) AddHighFromWorkload(name string, value string) {
	if l.denied(name) {
		return
	}
	l.AddHigh(name, value)
}

// AddAutoFromWorkload behaves like AddAuto for a tag name coming from
// workload-controlled metadata, dropping the denied ones.
func (l *TagList) AddAutoFromWorkload(name string, value string) {
	if l.denied(name) {
		return
	}
	l.AddAuto(name, value)
}

// denied reports whether the tag must be dropped, and logs it when it is.
func (l *TagList) denied(name string) bool {
	if !l.IsDenied(name) {
		return false
	}

	log.Debugf("Ignoring tag %q from workload metadata: it is listed in %s", name, denylistSetting)
	return true
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
		denylist:             l.denylist,  // read-only, can be shared
	}
}

func deepCopyMap(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	maps.Copy(out, in)
	return out
}
