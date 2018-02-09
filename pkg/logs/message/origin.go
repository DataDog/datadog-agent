// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Origin represents the Origin of a message
type Origin struct {
	Identifier  string
	LogSource   *config.LogSource
	Offset      int64
	Timestamp   string
	tags        []string
	tagsPayload []byte
}

// NewOrigin returns a new Origin
func NewOrigin() *Origin {
	return &Origin{}
}

// Tags returns the tags of the origin.
func (o *Origin) Tags() []string {
	return o.tags
}

// TagsPayload returns the raw tag payload of the origin.
func (o *Origin) TagsPayload() []byte {
	return o.tagsPayload
}

// SetTags sets the tags of the origin.
func (o *Origin) SetTags(tags []string, config *config.LogsConfig) {

	o.tags = tags
	o.tagsPayload = []byte{}

	if config.Source != "" {
		o.tags = append(o.tags, "source:"+config.Source)
		o.tagsPayload = append(o.tagsPayload, []byte("[dd ddsource=\""+config.Source+"\"]")...)
	}

	if config.SourceCategory != "" {
		o.tags = append(o.tags, "sourcecategory:"+config.SourceCategory)
		o.tagsPayload = append(o.tagsPayload, []byte("[dd ddsourcecategory=\""+config.SourceCategory+"\"]")...)
	}

	if config.Tags != "" {
		o.tags = append(o.tags, strings.Split(config.Tags, ",")...)
		tagstring := config.Tags
		if len(tags) > 0 {
			tagstring = tagstring + "," + strings.Join(tags, ",")
		}
		o.tagsPayload = append(o.tagsPayload, []byte("[dd ddtags=\""+tagstring+"\"]")...)
	}

	if len(o.tagsPayload) == 0 {
		o.tagsPayload = []byte{'-'}
	}

}
