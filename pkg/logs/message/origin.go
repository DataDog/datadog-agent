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
	shared       *TagSnapshot
	messageTags  []string
}

// NewOrigin returns a new Origin
func NewOrigin(source *sources.LogSource) *Origin {
	return &Origin{
		LogSource: source,
	}
}

// Tags returns the tags of the origin.
//
// The returned slice must not be modified by the caller.
func (o *Origin) Tags() []string {
	return o.tagsToStringArray()
}

// HasTag reports whether Tags would contain the lowercase tag, or tag is service:<Service()>, ignoring ASCII case and without allocating.
func (o *Origin) HasTag(tag string) bool {
	if o == nil || o.LogSource == nil {
		return false
	}
	if containsFold(o.tags, tag) || containsFold(o.LogSource.Config.Tags, tag) {
		return true
	}
	if tag == "" || tag[0] != 's' {
		return false
	}
	if service, ok := strings.CutPrefix(tag, "service:"); ok {
		return service != "" && equalFold(service, o.Service())
	}
	category, ok := strings.CutPrefix(tag, "sourcecategory:")
	return ok && category != "" && equalFold(category, o.LogSource.Config.SourceCategory)
}

func containsFold(tags []string, tag string) bool {
	for _, t := range tags {
		if equalFold(t, tag) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] && lowerASCII(a[i]) != lowerASCII(b[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// TransportTags returns tags after filtering for intake encoders. Tags remains unfiltered.
// The returned slice must not be modified by the caller.
func (o *Origin) TransportTags() []string {
	if o == nil || o.LogSource == nil {
		return nil
	}
	f, _ := o.LogSource.TagFilter()
	if f == nil {
		return o.Tags()
	}
	return f.Keep(o.tagsToStringArray())
}

// TagsPayload returns the raw tag payload of the origin.
func (o *Origin) TagsPayload(processingTags []string) []byte {
	return o.tagsPayload(nil, processingTags)
}

// TransportTagsPayload filters ddtags and ddsourcecategory. It never filters ddsource.
func (o *Origin) TransportTagsPayload(processingTags []string) []byte {
	if o == nil || o.LogSource == nil {
		return []byte{}
	}
	f, _ := o.LogSource.TagFilter()
	return o.tagsPayload(f, processingTags)
}

func (o *Origin) tagsPayload(f sources.TagFilter, processingTags []string) []byte {
	if o == nil || o.LogSource == nil {
		return []byte{}
	}

	var tagsPayload []byte

	source := o.Source()
	if source != "" {
		tagsPayload = append(tagsPayload, []byte("[dd ddsource=\""+source+"\"]")...)
	}
	sourceCategory := o.LogSource.Config.SourceCategory
	if sourceCategory != "" && (f == nil || f.RetainsTag("sourcecategory", sourceCategory)) {
		tagsPayload = append(tagsPayload, []byte("[dd ddsourcecategory=\""+sourceCategory+"\"]")...)
	}

	var tags []string
	tags = append(tags, o.LogSource.Config.Tags...)
	tags = append(tags, o.tags...)
	tags = append(tags, processingTags...)
	if f != nil {
		tags = f.Keep(tags)
	}

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
	o.shared = nil
	o.messageTags = nil
}

// SetTagsWithSnapshot sets tags, which must contain exactly the tags of shared plus messageTags.
func (o *Origin) SetTagsWithSnapshot(tags []string, shared *TagSnapshot, messageTags []string) {
	o.tags = tags
	o.shared = shared
	o.messageTags = messageTags
}

// TagSnapshot returns the shared tags plus the message's own tags and service, which together match like HasTag.
// service is empty when the snapshot already holds it. ok is false when no snapshot was set or the source
// config changed since it was taken.
func (o *Origin) TagSnapshot() (shared *TagSnapshot, messageTags []string, service string, ok bool) {
	if o == nil || o.shared == nil || o.LogSource == nil || !o.shared.describes(o.LogSource) {
		return nil, nil, "", false
	}
	if o.shared.service == "" {
		service = o.service
	}
	return o.shared, o.messageTags, service, true
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
