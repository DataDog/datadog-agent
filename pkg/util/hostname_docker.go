// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build linux windows darwin
// I don't think windows and darwin can actually be docker hosts
// but keeping it this way for build consistency (for now)

package util

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	log "github.com/cihub/seelog"
)

func getContainerHostname() (bool, string) {
	var name string
	if config.IsContainerized() {
		// Docker
		log.Debug("GetHostname trying Docker API...")
		if getDockerHostname, found := hostname.ProviderCatalog["docker"]; found {
			name, err := getDockerHostname(name)
			if err == nil && ValidHostname(name) == nil {
				return true, name
			} else if isKubernetes() {
				log.Debug("GetHostname trying k8s...")
				// TODO
			}
		}
	}

	return false, name
}
