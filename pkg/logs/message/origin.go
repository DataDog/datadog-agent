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
	// tagFilterOverride replaces the LogSource's filter when set.
	tagFilterOverride TagFilter
}

// NewOrigin returns a new Origin.
func NewOrigin(source *sources.LogSource) *Origin {
	return &Origin{LogSource: source}
}

// TagFilter drops tags that must not leave the Agent. nil means no filtering.
type TagFilter interface {
	// Apply returns the surviving tags without mutating or aliasing tags.
	Apply(tags []string) []string
}

// Tags returns the origin's tags, unfiltered. Agent-local consumers (e.g.
// anomaly detection) need every tag; encoders must use TransportTags.
//
// The returned slice must not be modified by the caller.
func (o *Origin) Tags() []string {
	return o.tagsToStringArray()
}

// TransportTags returns the origin's tags after the source's tag filter.
//
// The returned slice must not be modified by the caller.
func (o *Origin) TransportTags() []string {
	return o.applyTagFilters(o.tagsToStringArray())
}

// TransportTagsToString encodes TransportTags as a comma-separated string.
func (o *Origin) TransportTagsToString() string {
	tags := o.TransportTags()

	if len(tags) == 0 {
		return ""
	}

	return strings.Join(tags, ",")
}

// SetTagFilters overrides the filter this origin inherits from its LogSource.
func (o *Origin) SetTagFilters(f TagFilter) {
	o.tagFilterOverride = f
}

// TagFilters returns the filter applied on the intake path, if any.
func (o *Origin) TagFilters() TagFilter {
	if o == nil {
		return nil
	}
	if o.tagFilterOverride != nil {
		return o.tagFilterOverride
	}
	return o.LogSource.TagFilters()
}

func (o *Origin) applyTagFilters(tags []string) []string {
	f := o.TagFilters()
	if f == nil {
		return tags
	}
	return f.Apply(tags)
}

// retainsTag reports whether a single tag survives the filter. Used for values
// the intake receives as their own field instead of inside ddtags.
func (o *Origin) retainsTag(tag string) bool {
	f := o.TagFilters()
	if f == nil {
		return true
	}
	return len(f.Apply([]string{tag})) == 1
}

// TagsPayload returns the RFC5424 structured-data tag payload, with tag
// filtering applied. ddsource is not filtered; ddsourcecategory is, so that
// excluding `sourcecategory` drops it on this transport as it does on HTTP,
// where it travels inside ddtags.
func (o *Origin) TagsPayload(processingTags []string) []byte {
	if o == nil || o.LogSource == nil {
		return []byte{}
	}

	var tagsPayload []byte

	source := o.Source()
	if source != "" {
		tagsPayload = append(tagsPayload, []byte("[dd ddsource=\""+source+"\"]")...)
	}
	sourceCategory := o.LogSource.Config.SourceCategory
	if sourceCategory != "" && o.retainsTag("sourcecategory:"+sourceCategory) {
		tagsPayload = append(tagsPayload, []byte("[dd ddsourcecategory=\""+sourceCategory+"\"]")...)
	}

	var tags []string
	tags = append(tags, o.LogSource.Config.Tags...)
	tags = append(tags, o.tags...)
	tags = append(tags, processingTags...)
	tags = o.applyTagFilters(tags)

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

// TagsToString encodes Tags as a comma-separated string. Unfiltered; see Tags.
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
func (o *Origin) SetTags(tags []string) {
	o.tags = tags
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
