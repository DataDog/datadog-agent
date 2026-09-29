// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// Origin represents the Origin of a message
type Origin struct {
	Identifier string
	LogSource  *sources.LogSource
	Offset     string
	// FilePath is the concrete path to the file that the message originated from.
	// This is only populated for file and journald sources. It is used by the
	// auditor to store the file path when fingerprinting is enabled.
	FilePath     string
	Fingerprint  *types.Fingerprint
	service      string
	source       string
	mappedSource string
	tags         []string
	// snapshot is the write-once tag view built by BuildTagSnapshot.
	// nil means "no snapshot": Tags, TagsToString and TagsPayload fall back
	// to merging the live source config on every read. A non-nil snapshot
	// with zero tags is still a snapshot and is served as such.
	snapshot *tagSnapshot
}

// fallbackMergeHook, when set, is called every time Tags, TagsToString or
// TagsPayload merges the live source config instead of reading a snapshot.
// It is nil in production; tests set it to count fallback merges.
var fallbackMergeHook func()

// NewOrigin returns a new Origin
func NewOrigin(source *sources.LogSource) *Origin {
	return &Origin{
		LogSource: source,
	}
}

// Tags returns the tags of the origin, in JSON/protobuf order: attached tags,
// then "sourcecategory:X" if set, then the configured tags.
//
// When the origin carries a tag snapshot (see BuildTagSnapshot), the returned
// slice is shared: every call returns the same backing array, and other
// readers (encoders, possibly on other goroutines) read it too. Callers must
// not write to it, sort it in place or append to it expecting a private
// copy. A caller that keeps the slice past the call may keep the reference
// (the snapshot is never mutated), but must copy it before changing it.
func (o *Origin) Tags() []string {
	return o.tagsToStringArray()
}

// TagsPayload returns the raw tag payload of the origin.
func (o *Origin) TagsPayload(processingTags []string) []byte {
	if o == nil || o.LogSource == nil {
		return []byte{}
	}
	// The source stays live: remap_source can change it after the tailer.
	if o.snapshot != nil {
		return o.snapshot.payload(o.Source(), processingTags)
	}
	if fallbackMergeHook != nil {
		fallbackMergeHook()
	}

	var tagsPayload []byte

	source := o.Source()
	if source != "" {
		tagsPayload = append(tagsPayload, []byte("[dd ddsource=\""+source+"\"]")...)
	}
	sourceCategory := o.LogSource.Config.SourceCategory
	if sourceCategory != "" {
		tagsPayload = append(tagsPayload, []byte("[dd ddsourcecategory=\""+sourceCategory+"\"]")...)
	}

	var tags []string
	tags = append(tags, o.LogSource.Config.Tags...)
	tags = append(tags, o.tags...)
	tags = append(tags, processingTags...)

	if len(tags) > 0 {
		tagsPayload = append(tagsPayload, []byte("[dd ddtags=\""+strings.Join(tags, ",")+"\"]")...)
	}
	if len(tagsPayload) == 0 {
		tagsPayload = []byte{}
	}
	return tagsPayload
}

// TagMetadataBytes returns the byte length of comma-joined tag strings produced
// from the provided tag groups.
func TagMetadataBytes(tagGroups ...[]string) int {
	totalBytes := 0
	tagCount := 0
	for _, tags := range tagGroups {
		for _, tag := range tags {
			totalBytes += len(tag)
			tagCount++
		}
	}
	if tagCount > 1 {
		totalBytes += tagCount - 1
	}
	return totalBytes
}

// AppendTagMetadataBytes returns the tag metadata byte length after appending
// tags to an existing comma-joined tag metadata value.
func AppendTagMetadataBytes(baseBytes int, tags []string) int {
	totalBytes := baseBytes
	for _, tag := range tags {
		if totalBytes > 0 {
			totalBytes++
		}
		totalBytes += len(tag)
	}
	return totalBytes
}

// TagsToString encodes tags to a single string, in a comma separated format
func (o *Origin) TagsToString() string {
	tags := o.tagsToStringArray()

	if tags == nil {
		return ""
	}

	return strings.Join(tags, ",")
}

func (o *Origin) tagsToStringArray() []string {
	if o == nil || o.LogSource == nil {
		return nil
	}
	if o.snapshot != nil {
		return o.snapshot.merged
	}
	if fallbackMergeHook != nil {
		fallbackMergeHook()
	}
	sourceCategory := o.LogSource.Config.SourceCategory
	configTags := o.LogSource.Config.Tags

	// Calculate total capacity needed
	totalLen := len(o.tags) + len(configTags)
	if sourceCategory != "" {
		totalLen++
	}

	// Preallocate result slice - don't modify o.tags
	result := make([]string, 0, totalLen)
	result = append(result, o.tags...)

	if sourceCategory != "" {
		result = append(result, "sourcecategory:"+sourceCategory)
	}

	result = append(result, configTags...)

	return result
}

// SetTags sets the tags of the origin.
//
// It drops any tag snapshot built earlier, so a tag writer that runs after
// BuildTagSnapshot is never silently ignored: the origin falls back to
// merging the live source config until the snapshot is built again.
func (o *Origin) SetTags(tags []string) {
	o.tags = tags
	o.snapshot = nil
}

// BuildTagSnapshot freezes the origin's tag view: the tags passed to SetTags
// (attached), LogSource.Config.SourceCategory and LogSource.Config.Tags
// (configured). Tailers call it once, right before they send the message.
// After it, Tags, TagsToString and TagsPayload read the snapshot instead of
// the live source config, so a later in-place change to Config.Tags does not
// reach this message. Every input slice is copied.
//
// The source and the service are not frozen: Source and Service keep reading
// the live values (remap_source runs after the tailer).
//
// It is a no-op on a nil origin or an origin without a source config, which
// then keeps the fallback path.
func (o *Origin) BuildTagSnapshot() {
	if o == nil || o.LogSource == nil || o.LogSource.Config == nil {
		return
	}
	o.snapshot = newTagSnapshot(o.tags, o.LogSource.Config.Tags, o.LogSource.Config.SourceCategory)
}

// SetSource sets the source of the origin.
func (o *Origin) SetSource(source string) {
	o.source = source
}

// SetMappedSource sets a high-priority source override, typically from a
// remap_source processing rule. It takes precedence over both the config
// source and the parser-derived source.
func (o *Origin) SetMappedSource(source string) {
	o.mappedSource = source
}

// Source returns the source with the following priority:
//  1. mappedSource (set by remap_source processing rule)
//  2. LogSource.Config.Source (user-configured source)
//  3. o.source (parser-derived, e.g. syslog AppName)
func (o *Origin) Source() string {
	if o == nil || o.LogSource == nil {
		return ""
	}
	if o.mappedSource != "" {
		return o.mappedSource
	}
	if o.LogSource.Config.Source != "" {
		return o.LogSource.Config.Source
	}
	return o.source
}

// SetService sets the service of the origin.
func (o *Origin) SetService(service string) {
	o.service = service
}

// Service returns the service of the configuration if set or the service of the message,
// if none are defined, returns an empty string by default.
func (o *Origin) Service() string {
	if o == nil || o.LogSource == nil {
		return ""
	}
	if o.LogSource.Config.Service != "" {
		return o.LogSource.Config.Service
	}
	return o.service
}
