// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// NoopFilterList is a Noop FilterList to be used for tests.
type noopFilterList struct{}

func NewNoopFilterList() filterlist.Component {
	return &noopFilterList{}
}

// OnUpdateMetricFilterList does nothing.
func (*noopFilterList) OnUpdateMetricFilterList(_ func(utilstrings.Matcher, utilstrings.Matcher)) {}

// OnUpdateTagFilterList calls the callback with a noop tag matcher.
func (*noopFilterList) OnUpdateTagFilterList(onUpdate func(filterlist.TagMatcher)) {
	onUpdate(NewNoopTagMatcher())
}

// GetTagFilterList does nothing.
func (*noopFilterList) GetTagFilterList() filterlist.TagMatcher {
	return NewNoopTagMatcher()
}

// GetTagFilterList does nothing.
func (*noopFilterList) GetMetricFilterList() utilstrings.Matcher {
	return utilstrings.NewMatcher([]string{}, false)
}

type noopTagMatcher struct{}

func NewNoopTagMatcher() filterlist.TagMatcher {
	return &noopTagMatcher{}
}

func (*noopTagMatcher) ShouldStripTags(_ string) (func(tag string) bool, bool) {
	return func(_ string) bool {
		return true
	}, false
}
