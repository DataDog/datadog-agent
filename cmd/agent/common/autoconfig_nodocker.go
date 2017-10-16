// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !docker

package common

import (
	log "github.com/cihub/seelog"
)

// SetupAutoConfig placeholder if docker is disabled
func SetupAutoConfig(confdPath string) {
	log.Debugf("AutoDiscovery is only supported with docker, disabling")
}

// StartAutoConfig placeholder if docker is disabled
func StartAutoConfig() {
}
