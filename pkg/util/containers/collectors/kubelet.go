// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet

package collectors

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const (
	kubeletCollectorName = "kubelet"
)

// KubeletCollector lists containers from the kubelet podlist and populates
// performance metric from the linux cgroups
type KubeletCollector struct {
	kubeUtil kubelet.KubeUtilInterface
}

// Detect tries to connect to the kubelet
func (c *KubeletCollector) Detect() error {
	if !config.IsFeaturePresent(config.Kubernetes) {
		return errors.New("Kubernetes feature is deactivated")
	}

	util, err := kubelet.GetKubeUtil()
	if err != nil {
		return err
	}
	c.kubeUtil = util
	return nil
}

// List gets all running containers
func (c *KubeletCollector) List() ([]*containers.Container, error) {
	return c.kubeUtil.ListContainers(context.TODO())
}

// UpdateMetrics updates metrics on an existing list of containers
func (c *KubeletCollector) UpdateMetrics(cList []*containers.Container) error {
	return c.kubeUtil.UpdateContainerMetrics(cList)
}

func kubeletFactory() Collector {
	return &KubeletCollector{}
}

func init() {
	registerCollector(kubeletCollectorName, kubeletFactory, NodeOrchestrator)
}
