// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

// Package podtagprovider implements a provider that supplies pod tags for cluster checks
package podtagprovider

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	corev1 "k8s.io/api/core/v1"
)

// PodTagProvider can be used to extract pod tags
type PodTagProvider interface {
	GetTags(*corev1.Pod, taggertypes.TagCardinality) ([]string, error)
}

// NewPodTagProvider returns a new PodTagProvider
// In case of CLC runner, the provider will calculate tags based on the pod resource on demand each time
// In case of Node agent or Cluster agent, the provider will get pod tags by querying the tagger
func NewPodTagProvider(cfg config.Component, store workloadmeta.Component, tagger tagger.Component) PodTagProvider {
	if pkgconfigsetup.IsCLCRunner(pkgconfigsetup.Datadog()) {
		// Running in a CLC Runner
		return newCLCTagProvider(cfg, store)
	}

	// Running in the Node Agent or in the Cluster Agent
	return newNodePodTagProvider(tagger)
}
