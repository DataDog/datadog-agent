// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infraattributesprocessor

import "github.com/DataDog/datadog-agent/comp/core/tagger/types"

// taggerClient provides client for tagger interface,
// see comp/core/tagger for tagger functions; client for tagger interface
type taggerClient interface {
	// Tag is an interface function that queries taggerclient singleton
	Tag(entity types.EntityID, cardinality types.TagCardinality) ([]string, error)
	// GlobalTags is an interface function that queries taggerclient singleton
	GlobalTags(cardinality types.TagCardinality) ([]string, error)
}
