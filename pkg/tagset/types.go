// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

// A TagsAccumulator accumulates tags.  The underlying type will provide a means of getting
// the resulting tag set.
type TagsAccumulator interface {
	// Append the given tags to the tag set
	Append(...string)
	// Append the tags contained in the given HashedTags instance to the tag set.
	AppendHashed(HashedTags)
}
