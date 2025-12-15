// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/tagset"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// tagStripAccumulator uses the tagFilterlist to strip any unwanted tags appended
// whilst accumulating.
type tagStripAccumulator struct {
	accumulator   tagset.TagsAccumulator
	tagFilterList utilstrings.TagMatcher
}

func (t *tagStripAccumulator) Append(tags ...string) {
	newTags, _ := t.tagFilterList.StripTagsMut(tags)
	t.accumulator.Append(newTags...)
}

func (t *tagStripAccumulator) AppendHashed(tags tagset.HashedTags) {
	newTags, stripped := t.tagFilterList.StripTags(tags.Get())
	if stripped {
		// Sadly we have to recalculate the hash if the tags are changed.
		t.accumulator.Append(newTags...)
	} else {
		t.accumulator.AppendHashed(tags)
	}
}
