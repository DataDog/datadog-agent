// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"context"
	"strings"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	tagNames "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var tagSet = map[string]struct{}{
	tagNames.Env:              {},
	tagNames.ClusterName:      {},
	tagNames.KubeClusterName:  {},
	tagNames.OrchClusterID:    {},
	tagNames.KubeDistribution: {},
	tagNames.KubeNamespace:    {},
}

// TagsProvider builds connection tags
type TagsProvider interface {
	GetTags(ctx context.Context, runnerID, hostname string) []string
}

type taggerBasedTagsProvider struct {
	tagger tagger.Component
}

// NewTagsProvider creates a TagsProvider that uses the tagger component to get cluster tags
func NewTagsProvider(tagger tagger.Component) TagsProvider {
	return &taggerBasedTagsProvider{
		tagger,
	}
}

func (p *taggerBasedTagsProvider) GetTags(ctx context.Context, runnerID, hostname string) []string {
	tags := []string{
		"runner-id:" + runnerID,
		"hostname:" + hostname,
	}

	// Only attempt to get cluster tags if cluster_agent is enabled
	globalTags, err := p.tagger.GlobalTags(types.LowCardinality)
	if err != nil {
		log.Debugf("Failed to get global tags from tagger: %v", err)
		return tags
	}

	for _, keyValue := range globalTags {
		tagName, _, ok := strings.Cut(keyValue, ":")
		if ok {
			if _, found := tagSet[tagName]; found {

				tags = append(tags, keyValue)
			}
		}
	}

	return tags
}
