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
	Identifier string
	LogSource  *config.LogSource
	Offset     string
	service    string
	source     string
	tags       []string
}

// NewOrigin returns a new Origin
func NewOrigin(source *config.LogSource) *Origin {
	return &Origin{
		LogSource: source,
	}
}

// Tags returns the tags of the origin merged with those from the LogSource config
//
// The returned slice must not be modified by the caller.
func (o *Origin) Tags() []string {
	return o.tagsToStringArray(true)
}

// TagsPayload returns the raw tag payload of the origin (including those from the LogSource config).
func (o *Origin) TagsPayload() []byte {
	var tagsPayload []byte

	source := o.Source()
	if source != "" {
		tagsPayload = append(tagsPayload, []byte("[dd ddsource=\""+source+"\"]")...)
	}
	sourceCategory := o.LogSource.Config.SourceCategory
	if sourceCategory != "" {
		tagsPayload = append(tagsPayload, []byte("[dd ddsourcecategory=\""+sourceCategory+"\"]")...)
	}

	tags := o.tagsToStringArray(false)

	if len(tags) > 0 {
		tagsPayload = append(tagsPayload, []byte("[dd ddtags=\""+strings.Join(tags, ",")+"\"]")...)
	}
	if len(tagsPayload) == 0 {
		tagsPayload = []byte{}
	}
	return tagsPayload
}

// TagsToString encodes tags of the origin (including those from the LogSource config) to a single string, in a comma separated format
func (o *Origin) TagsToString() string {
	tags := o.tagsToStringArray(true)

	if tags == nil {
		return ""
	}

	return strings.Join(tags, ",")
}

func (o *Origin) tagsToStringArray(addSourceCategory bool) []string {
	tmpMap := make(map[string]struct{}, len(o.tags)+len(o.LogSource.Config.Tags)+1)

	for i := range o.tags {
		tmpMap[o.tags[i]] = struct{}{}
	}
	sourceCategory := o.LogSource.Config.SourceCategory
	if addSourceCategory && sourceCategory != "" {
		tmpMap["sourcecategory"+":"+sourceCategory] = struct{}{}
	}

	for i := range o.LogSource.Config.Tags {
		tmpMap[o.LogSource.Config.Tags[i]] = struct{}{}
	}

	tags := make([]string, 0, len(tmpMap))
	for k := range tmpMap {
		tags = append(tags, k)
	}
	return tags
}

// SetTags sets the tags of the origin.
func (o *Origin) SetTags(tags []string) {
	o.tags = tags
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
