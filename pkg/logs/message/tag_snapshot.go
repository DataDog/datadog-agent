// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

// sourceCategoryTagPrefix is how JSON and protobuf carry the source category
// inline in the tag list.
const sourceCategoryTagPrefix = "sourcecategory:"

// tagSnapshot is the tag view a tailer freezes right before it sends a
// message. It keeps the three tag groups separate because the encoders need
// them in different shapes:
//
//   - JSON / protobuf read merged: attached..., "sourcecategory:X" (if set),
//     configured...
//   - raw reads the groups: [dd ddsourcecategory="X"] as its own element and
//     ddtags="configured...,attached...,processing..."
//
// A tagSnapshot is write-once: it is built by (*Origin).BuildTagSnapshot and
// never mutated afterwards. Origin.SetTags drops it (the Origin falls back to
// the live merge) instead of editing it. Readers may therefore share the
// slices it hands out, across goroutines, without copying, as long as they
// do not write to them.
//
// The source and the service are deliberately not part of the snapshot: the
// processor can remap the source after the tailer (remap_source), and the
// raw encoder must emit the mapped value.
//
// It costs one slice allocation: attached and configured are capped
// sub-slices of merged, so an append on either can never write into merged.
//
// Note for the adaptive sampler move (PR4): sampler tags (noisy_log, log_hash,
// adaptive_sampler_sampled_count) ride in ParsingExtra.Tags today, so they
// land in attached, and their position differs by family. The container
// tailer attaches parsing tags before provider tags (sampler tags come before
// container_name:...), while the file tailer attaches path and provider tags
// first (sampler tags come after them). Moving the sampler past the tailer
// needs a separate result-tag group with a fixed wire position; choosing that
// position decides whether one of the two families changes order. That
// decision is left to PR4. Until then a post-snapshot SetTags stays correct
// because it invalidates the snapshot.
type tagSnapshot struct {
	// merged is the JSON/protobuf tag list. Never nil, even when empty.
	merged []string
	// attached is merged[:len(attached):len(attached)]: the tags the tailer
	// passed to Origin.SetTags (parsing, provider and static/entry tags).
	attached []string
	// configured is the tail of merged: a copy of LogSource.Config.Tags.
	configured []string
	// sourceCategory is a copy of LogSource.Config.SourceCategory.
	sourceCategory string
}

// newTagSnapshot builds a snapshot from the given groups. It copies both
// slices, so later writes to the caller's slices do not reach the snapshot.
func newTagSnapshot(attached, configured []string, sourceCategory string) *tagSnapshot {
	total := len(attached) + len(configured)
	if sourceCategory != "" {
		total++
	}

	merged := make([]string, 0, total)
	merged = append(merged, attached...)
	if sourceCategory != "" {
		merged = append(merged, sourceCategoryTagPrefix+sourceCategory)
	}
	merged = append(merged, configured...)

	nAttached := len(attached)
	return &tagSnapshot{
		merged:         merged,
		attached:       merged[:nAttached:nAttached],
		configured:     merged[total-len(configured) : total : total],
		sourceCategory: sourceCategory,
	}
}

// payload returns the raw (syslog) structured-data tag payload. It is
// byte-identical to the fallback in Origin.TagsPayload: ddsource if source is
// set, ddsourcecategory if set, then ddtags with configured, attached and
// processing tags if any. It never returns nil.
func (s *tagSnapshot) payload(source string, processingTags []string) []byte {
	const (
		sourceOpen   = `[dd ddsource="`
		categoryOpen = `[dd ddsourcecategory="`
		tagsOpen     = `[dd ddtags="`
		elemClose    = `"]`
	)
	groups := [3][]string{s.configured, s.attached, processingTags}

	size := 0
	if source != "" {
		size += len(sourceOpen) + len(source) + len(elemClose)
	}
	if s.sourceCategory != "" {
		size += len(categoryOpen) + len(s.sourceCategory) + len(elemClose)
	}
	tagCount := 0
	for _, group := range groups {
		for _, tag := range group {
			size += len(tag)
			tagCount++
		}
	}
	if tagCount > 0 {
		size += len(tagsOpen) + tagCount - 1 + len(elemClose)
	}
	if size == 0 {
		return []byte{}
	}

	payload := make([]byte, 0, size)
	if source != "" {
		payload = append(payload, sourceOpen...)
		payload = append(payload, source...)
		payload = append(payload, elemClose...)
	}
	if s.sourceCategory != "" {
		payload = append(payload, categoryOpen...)
		payload = append(payload, s.sourceCategory...)
		payload = append(payload, elemClose...)
	}
	if tagCount > 0 {
		payload = append(payload, tagsOpen...)
		first := true
		for _, group := range groups {
			for _, tag := range group {
				if !first {
					payload = append(payload, ',')
				}
				payload = append(payload, tag...)
				first = false
			}
		}
		payload = append(payload, elemClose...)
	}
	return payload
}
