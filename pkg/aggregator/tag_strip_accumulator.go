// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/tagset"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

type tagStripAccumulator struct {
	currentName string
	accumulator tagset.TagsAccumulator
	filterList  *utilstrings.Matcher
}

func (t *tagStripAccumulator) Append(tags ...string) {
	newTags, _ := t.filterList.StripTags(t.currentName, tags)
	t.accumulator.Append(newTags...)
}

func (t *tagStripAccumulator) AppendHashed(tags tagset.HashedTags) {
	newTags, stripped := t.filterList.StripTags(t.currentName, tags.Get())
	if stripped {
		// Sadly we have to recalculate the hash if the tags are changed.
		t.Append(newTags...)
	} else {
		t.accumulator.AppendHashed(tags)
	}
}
