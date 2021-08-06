// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux windows darwin
// I don't think windows and darwin can actually be docker hosts
// but keeping it this way for build consistency (for now)

package util

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getContainerHostname(ctx context.Context) (bool, string) {
	var name string

	if config.IsFeaturePresent(config.Kubernetes) {
		// Cluster-agent logic: Kube apiserver
		if getKubeHostname, found := hostname.ProviderCatalog["kube_apiserver"]; found {
			log.Debug("GetHostname trying Kubernetes trough API server...")
			name, err := getKubeHostname(ctx, nil)
			if err == nil && validate.ValidHostname(name) == nil {
				return true, name
			}
		}
	}

	// Node-agent logic: docker or kubelet
	if config.IsFeaturePresent(config.Docker) {
		log.Debug("GetHostname trying Docker API...")
		if getDockerHostname, found := hostname.ProviderCatalog["docker"]; found {
			name, err := getDockerHostname(ctx, nil)
			if err == nil && validate.ValidHostname(name) == nil {
				return true, name
			}
		}
	}

	if config.IsFeaturePresent(config.Kubernetes) {
		if getKubeletHostname, found := hostname.ProviderCatalog["kubelet"]; found {
			log.Debug("GetHostname trying Kubernetes trough kubelet API...")
			name, err := getKubeletHostname(ctx, nil)
			if err == nil && validate.ValidHostname(name) == nil {
				return true, name
			}
		}
	}

	return false, name
}
