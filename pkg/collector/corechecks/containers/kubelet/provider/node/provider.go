// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package node is responsible for emitting the Kubelet check metrics that are
// collected from the `/spec` endpoint.
package node

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// NOTE: this provider is here for backwards compatibility with k8s <1.19. This endpoint was hidden in k8s 1.18 and removed
// in k8s 1.19.

type nodeSpec struct {
	NumCores       float64 `json:"num_cores"`
	MemoryCapacity float64 `json:"memory_capacity"`
}

// Provider provides the metrics related to data collected from the `/spec/` Kubelet endpoint
type Provider struct {
	config *common.KubeletConfig
}

// NewProvider returns a new Provider
func NewProvider(config *common.KubeletConfig) *Provider {
	return &Provider{
		config: config,
	}
}

// Provide sends the metrics collected from the `/spec` Kubelet endpoint
func (p *Provider) Provide(kc kubelet.KubeUtilInterface, sender sender.Sender) error {
	// Collect raw data
	nodeSpecRaw, responseCode, err := kc.QueryKubelet(context.TODO(), "/spec/")
	if err != nil || responseCode == 404 {
		if responseCode == 404 {
			return nil
		}
		return err
	}

	var node *nodeSpec
	err = json.Unmarshal(nodeSpecRaw, &node)
	if err != nil {
		return err
	}

	// Report metrics
	sender.Gauge(common.KubeletMetricsPrefix+"cpu.capacity", node.NumCores, "", p.config.Tags)
	sender.Gauge(common.KubeletMetricsPrefix+"memory.capacity", node.MemoryCapacity, "", p.config.Tags)

	return nil
}
