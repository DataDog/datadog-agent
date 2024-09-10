// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package podtagprovider

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"

	corev1 "k8s.io/api/core/v1"
)

// podTagsProvider is used to get tags for the
type clcTagProvider struct {
	parser    kubernetesresourceparsers.ObjectParser
	extractor collectors.PodTagExtractor
}

func newCLCTagProvider(cfg config.Component, store workloadmeta.Component) PodTagProvider {
	parser, _ := kubernetesresourceparsers.NewPodParser(cfg.GetStringSlice("cluster_agent.kubernetes_resources_collection.pod_annotations_exclude"))
	extractor := *collectors.NewPodTagExtractor(cfg, store)

	return &clcTagProvider{
		parser:    parser,
		extractor: extractor,
	}
}

// GetTags implements PodTagsProvider#GetTags
func (p *clcTagProvider) GetTags(pod *corev1.Pod, cardinality taggertypes.TagCardinality) ([]string, error) {
	parsedEntity := p.parser.Parse(pod)
	podEntity, _ := parsedEntity.(*workloadmeta.KubernetesPod)
	taggerTags := p.extractor.Extract(podEntity, cardinality)
	return taggerTags, nil
}
