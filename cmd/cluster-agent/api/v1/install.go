// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package v1 implements the v1 of the API that exposes methods to manage and
// get information about the cluster checks and the endpoint checks dispatched
// by the cluster-agent.
package v1

import (
	"context"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
)

// InstallMetadataEndpoints registers endpoints for metadata
func InstallMetadataEndpoints(r *mux.Router, w workloadmeta.Component) {
	log.Debug("Registering metadata endpoints")
	if pkgconfigsetup.Datadog().GetBool("cloud_foundry") {
		installCloudFoundryMetadataEndpoints(r)
	} else {
		installKubernetesMetadataEndpoints(r, w)
	}
}

func setupClusterCheck(ctx context.Context, ac autodiscovery.Component, tagger tagger.Component) (*clusterchecks.Handler, error) {
	handler, err := clusterchecks.NewHandler(ac, tagger)
	if err != nil {
		return nil, err
	}
	go func() {
		handler.Run(ctx)
		log.Info("Stopped cluster check Autodiscovery")
	}()

	log.Info("Started cluster check Autodiscovery")
	return handler, nil
}

// InstallChecksEndpoints registers endpoints for cluster checks
func InstallChecksEndpoints(ctx context.Context, r *mux.Router, ac autodiscovery.Component, tagger tagger.Component) {
	log.Debug("Registering checks endpoints")
	clusterCheckHandler, err := setupClusterCheck(ctx, ac, tagger)
	if err != nil {
		log.Errorf("Error while setting up cluster check Autodiscovery, CLC API endpoints won't be available, err: %v", err)
		return
	}
	sc := clusteragent.ServerContext{ClusterCheckHandler: clusterCheckHandler}
	installClusterCheckEndpoints(r, sc)
	installEndpointsCheckEndpoints(r, sc)
}
