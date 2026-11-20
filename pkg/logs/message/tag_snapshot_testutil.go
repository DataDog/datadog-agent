// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package message

import (
	"errors"
	"fmt"
	"slices"
)

// CheckTagSnapshot returns an error unless o has a snapshot that, with its message tags, holds exactly o's tags,
// and the message tags are a subslice of the tags o encodes.
func CheckTagSnapshot(o *Origin) error {
	shared, msgTags, _, ok := o.TagSnapshot()
	if !ok {
		return errors.New("origin has no usable tag snapshot")
	}
	if len(msgTags) > 0 && !isSubslice(o.tags, msgTags) {
		return fmt.Errorf("message tags %v are not a subslice of origin tags %v", msgTags, o.tags)
	}
	got := slices.Compact(slices.Sorted(slices.Values(append(slices.Collect(shared.All()), msgTags...))))
	want := o.Tags()
	if service := o.LogSource.Config.Service; service != "" {
		want = append(want, "service:"+service)
	}
	want = slices.Compact(slices.Sorted(slices.Values(want)))
	if !slices.Equal(got, want) {
		return fmt.Errorf("snapshot tags %v, origin tags %v", got, want)
	}
	return nil
}

func isSubslice(s, sub []string) bool {
	for i := range s {
		if &s[i] == &sub[0] {
			return len(s)-i >= len(sub)
		}
	}
	return false
}
