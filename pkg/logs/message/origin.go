// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Origin represents the Origin of a message
type Origin struct {
	Identifier    string
	LogSource     *config.LogSource
	Offset        string
	service       string
	source        string
	tags          []string
	logSourceTags map[string]struct{}
}

// NewOrigin returns a new Origin
func NewOrigin(source *config.LogSource) *Origin {
	if source == nil {
		return &Origin{
			LogSource: source,
		}
	}

	logSourceTags := make(map[string]struct{}, len(source.Config.Tags))
	for i := range source.Config.Tags {
		logSourceTags[source.Config.Tags[i]] = struct{}{}
	}
	return &Origin{
		LogSource:     source,
		logSourceTags: logSourceTags,
	}
}

// Tags returns the tags of the origin.
//
// The returned slice must not be modified by the caller.
func (o *Origin) Tags() []string {
	return o.tagsToStringArray(true)
}

// TagsPayload returns the raw tag payload of the origin.
func (o *Origin) TagsPayload() []byte {
	var tagsPayload strings.Builder

	source := o.Source()
	if source != "" {
		tagsPayload.WriteString("[dd ddsource=\"")
		tagsPayload.WriteString(source)
		tagsPayload.WriteString("\"]")
	}
	sourceCategory := o.LogSource.Config.SourceCategory
	if sourceCategory != "" {
		tagsPayload.WriteString("[dd ddsourcecategory=\"")
		tagsPayload.WriteString(sourceCategory)
		tagsPayload.WriteString("\"]")
	}

	tags := o.tagsToStringArray(false)
	if len(tags) > 0 {
		tagsPayload.WriteString("[dd ddtags=\"")
		tagsPayload.WriteString(strings.Join(tags, ","))
		tagsPayload.WriteString("\"]")
	}
	return []byte(tagsPayload.String())
}

// TagsToString encodes tags to a single string, in a comma separated format
func (o *Origin) TagsToString() string {
	tags := o.tagsToStringArray(true)

	if tags == nil {
		return ""
	}

	return strings.Join(tags, ",")
}

func (o *Origin) tagsToStringArray(incSource bool) []string {
	tags := make([]string, 0, len(o.tags)+len(o.logSourceTags)+1)
	tags = append(tags, o.tags...)
	sourceCategory := o.LogSource.Config.SourceCategory
	if sourceCategory != "" && incSource {
		tags = append(tags, "sourcecategory"+":"+sourceCategory)
	}

	for key := range o.logSourceTags {
		tags = append(tags, key)
	}
	return tags
}

// SetTags sets the extra tags of the origin.
// These tags are combined with those from the log source when message sent.
func (o *Origin) SetTags(tags []string) {
	filteredTags := make([]string, 0, len(tags))
	for i := range tags {
		if _, ok := o.logSourceTags[tags[i]]; !ok {
			filteredTags = append(filteredTags, tags[i])
		}
	}
	o.tags = filteredTags
}

// SetSource sets the source of the origin.
func (o *Origin) SetSource(source string) {
	o.source = source
}

// Source returns the source of the configuration if set or the source of the message,
// if none are defined, returns an empty string by default.
func (o *Origin) Source() string {
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
	if o.LogSource.Config.Service != "" {
		return o.LogSource.Config.Service
	}
	return o.service
}
