// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubelet

package kubernetes

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
)

var (
	// CloudProviderName contains the inventory name for Kubernetes (through the API server)
	CloudProviderName = "kubernetes"
)

// GetHostAliases returns the host aliases from the Kubernetes node annotations
func GetHostAliases(ctx context.Context) ([]string, error) {
	if !config.IsFeaturePresent(config.Kubernetes) {
		return []string{}, nil
	}

	aliases := []string{}

	annotations, err := hostinfo.GetNodeAnnotations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get node annotations: %w", err)
	}

	for _, annotation := range config.Datadog.GetStringSlice("kubernetes_node_annotations_as_host_aliases") {
		if value, found := annotations[annotation]; found {
			aliases = append(aliases, value)
		}
	}

	return aliases, nil
}
