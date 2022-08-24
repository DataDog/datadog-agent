// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1

import (
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
)

// InstallMetadataEndpoints registers endpoints for metadata
func InstallMetadataEndpoints(r *mux.Router) {
	log.Debug("Registering metadata endpoints")
	if config.Datadog.GetBool("cloud_foundry") {
		installCloudFoundryMetadataEndpoints(r)
	} else {
		installKubernetesMetadataEndpoints(r)
	}
}

// InstallChecksEndpoints registers endpoints for cluster checks
func InstallChecksEndpoints(r *mux.Router, sc clusteragent.ServerContext) {
	log.Debug("Registering checks endpoints")
	installClusterCheckEndpoints(r, sc)
	installEndpointsCheckEndpoints(r, sc)
}
