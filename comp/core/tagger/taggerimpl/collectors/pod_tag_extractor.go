// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PodTagExtractor is used to extract tags from pod entity
type PodTagExtractor struct {
	c WorkloadMetaCollector
}

// Extract extracts and returns tags from a workloadmeta pod entity
func (p *PodTagExtractor) Extract(podEntity *workloadmeta.KubernetesPod, cardinality types.TagCardinality) []string {
	tagInfos := p.c.extractTagsFromPodEntity(podEntity, taglist.NewTagList())

	switch cardinality {
	case types.HighCardinality:
		tags := tagInfos.LowCardTags
		tags = append(tags, tagInfos.HighCardTags...)
		tags = append(tags, tagInfos.OrchestratorCardTags...)
		return tags
	case types.OrchestratorCardinality:
		return append(tagInfos.LowCardTags, tagInfos.OrchestratorCardTags...)
	case types.LowCardinality:
		return tagInfos.LowCardTags
	default:
		log.Errorf("unsupported tag cardinality %v", cardinality)
		return []string{}
	}
}

// NewPodTagExtractor creates a new Pod Tag Extractor
func NewPodTagExtractor(cfg config.Component, store workloadmeta.Component) *PodTagExtractor {
	return &PodTagExtractor{
		c: *NewWorkloadMetaCollector(context.Background(), cfg, store, nil),
	}
}
