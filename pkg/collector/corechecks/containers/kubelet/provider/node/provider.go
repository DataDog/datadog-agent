// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package node

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// NOTE: this provider is here for backwards compatibility with k8s <1.19. This endpoint was hidden in k8s 1.18 and removed
// in k8s 1.19.

type nodeSpec struct {
	NumCores       float64 `json:"num_cores"`
	MemoryCapacity float64 `json:"memory_capacity"`
}

type Provider struct {
	config *common.KubeletConfig
}

func NewProvider(config *common.KubeletConfig) *Provider {
	return &Provider{
		config: config,
	}
}

func (p *Provider) Collect(kc kubelet.KubeUtilInterface) (interface{}, error) {
	nodeSpecRaw, responseCode, err := kc.QueryKubelet(context.TODO(), "/spec/")
	if err != nil || responseCode == 404 {
		if responseCode == 404 {
			return nil, nil
		}
		return nil, err
	}

	var spec *nodeSpec
	err = json.Unmarshal(nodeSpecRaw, &spec)
	if err != nil {
		return nil, err
	}

	return spec, nil
}

func (p *Provider) Transform(spec interface{}, sender aggregator.Sender) error {
	node, ok := spec.(*nodeSpec)
	if !ok || node == nil {
		return nil
	}

	sender.Gauge(common.KubeletMetricsPrefix+"cpu.capacity", node.NumCores, "", p.config.Tags)
	sender.Gauge(common.KubeletMetricsPrefix+"memory.capacity", node.MemoryCapacity, "", p.config.Tags)

	return nil
}
