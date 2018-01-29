// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build linux windows darwin
// I don't think windows and darwin can actually be docker hosts
// but keeping it this way for build consistency (for now)

package util

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func getContainerHostname() (bool, string) {
	var name string

	if config.IsContainerized() == false {
		return false, name
	}

	// Docker
	log.Debug("GetHostname trying Docker API...")
	if getDockerHostname, found := hostname.ProviderCatalog["docker"]; found {
		name, err := getDockerHostname(name)
		if err == nil && ValidHostname(name) == nil {
			return true, name
		}
	}

	if config.IsKubernetes() == false {
		return false, name
	}
	// Kubernetes
	log.Debug("GetHostname trying Kubernetes trough kubelet API...")
	if getKubeletHostname, found := hostname.ProviderCatalog["kubelet"]; found {
		name, err := getKubeletHostname(name)
		if err == nil && ValidHostname(name) == nil {
			return true, name
		}
	}
	return false, name
}
