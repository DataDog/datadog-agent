// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

package fx

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	identitystore "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/def"
	filestoreimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/impl-file"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module provides the identity store component (file-based only for non-kubeapiserver builds)
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newIdentityStore),
	)
}

// Requires defines dependencies for the selector
type Requires struct {
	compdef.In

	Config config.Component
	Log    log.Component
}

// Provides defines the selected implementation
type Provides struct {
	compdef.Out

	Comp identitystore.Component
}

// newIdentityStore creates a file-based identity store (K8s not available in this build)
func newIdentityStore(reqs Requires) (Provides, error) {
	if reqs.Config.GetBool("private_action_runner.use_k8s_secret") {
		reqs.Log.Warn("Kubernetes secret store requested but not available in this build (kubeapiserver build tag required). Using file-based identity store.")
	} else {
		reqs.Log.Info("Using file-based identity store for PAR")
	}

	fileReqs := filestoreimpl.Requires{
		Config: reqs.Config,
		Log:    reqs.Log,
	}
	return Provides{Comp: filestoreimpl.NewComponent(fileReqs).Comp}, nil
}
