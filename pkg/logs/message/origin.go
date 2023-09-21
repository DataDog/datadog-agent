// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message/module"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Origin represents the Origin of a message
type Origin module.Origin

// NewOrigin returns a new Origin
func NewOrigin(source *sources.LogSource) *module.Origin {
	return &module.Origin{
		LogSource: source,
	}
}

// Tags returns the tags of the origin.
//
// The returned slice must not be modified by the caller.
var Tags = (*module.Origin).Tags

// TagsPayload returns the raw tag payload of the origin.
var TagsPayload = (*module.Origin).TagsPayload

// TagsToString encodes tags to a single string, in a comma separated format
var TagsToString = (*module.Origin).TagsToString

// SetTags sets the tags of the origin.
var SetTags = (*module.Origin).SetTags

// SetSource sets the source of the origin.
var SetSource = (*module.Origin).SetSource

// Source returns the source of the configuration if set or the source of the message,
// if none are defined, returns an empty string by default.
var Source = (*module.Origin).Source

// SetService sets the service of the origin.
var SetService = (*module.Origin).SetService

// Service returns the service of the configuration if set or the service of the message,
// if none are defined, returns an empty string by default.
var Service = (*module.Origin).Service
