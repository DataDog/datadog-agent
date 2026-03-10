// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package catalog is the workloadmeta collector catalog for the otel-agent.
// It includes collectors for Kubernetes (kubelet) and container runtimes so that
// the local tagger can enrich OTel spans/metrics/logs with K8s entity tags.
package catalog

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/containerd"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/crio"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/docker"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/ecs"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubelet"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/podman"
)

// GetCatalog returns the FX options for the otel-agent workloadmeta collectors.
func GetCatalog() fx.Option {
	options := []fx.Option{
		containerd.GetFxOptions(),
		crio.GetFxOptions(),
		docker.GetFxOptions(),
		ecs.GetFxOptions(),
		kubelet.GetFxOptions(),
		podman.GetFxOptions(),
	}

	// remove nil options (collectors disabled by build tags)
	opts := make([]fx.Option, 0, len(options))
	for _, item := range options {
		if item != nil {
			opts = append(opts, item)
		}
	}
	return fx.Options(opts...)
}
