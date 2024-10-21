// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package podtagprovider

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

type nodePodTagProvider struct{}

func newNodePodTagProvider() PodTagProvider { return &nodePodTagProvider{} }

// GetTags implements PodTagProvider#GetTags
func (p *nodePodTagProvider) GetTags(pod *corev1.Pod, cardinality taggertypes.TagCardinality) ([]string, error) {
	return tagger.Tag(taggertypes.NewEntityID(taggertypes.KubernetesPodUID, string(pod.UID)).String(), cardinality)
}
