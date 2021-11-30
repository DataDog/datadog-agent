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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getContainerHostname(ctx context.Context) string {
	if config.IsFeaturePresent(config.Kubernetes) {
		// Cluster-agent logic: Kube apiserver
		name, err := hostname.GetHostname(ctx, "kube_apiserver", nil)
		if err == nil {
			return name
		}
		log.Debug(err.Error())
	}

	// Node-agent logic: docker or kubelet
	if config.IsFeaturePresent(config.Docker) {
		name, err := hostname.GetHostname(ctx, "docker", nil)
		if err == nil {
			return name
		}
		log.Debug(err.Error())
	}

	if config.IsFeaturePresent(config.Kubernetes) {
		name, err := hostname.GetHostname(ctx, "kubelet", nil)
		if err == nil {
			return name
		}
		log.Debug(err.Error())
	}

	return ""
}
