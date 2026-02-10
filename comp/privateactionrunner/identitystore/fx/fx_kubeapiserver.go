// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package fx

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	identitystore "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/def"
	fileimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/impl-file"
	k8simpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/impl-k8s"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module provides the identity store component with smart selection
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

// newIdentityStore selects the appropriate implementation based on configuration
func newIdentityStore(reqs Requires) (Provides, error) {
	storeKind := reqs.Config.GetString("private_action_runner.identity_store.kind")
	if storeKind == "k8s-secret" {
		reqs.Log.Info("Using Kubernetes secret-based identity store for PAR")
		return createKubeAPIServerStore(reqs)
	}
	reqs.Log.Info("Using file-based identity store for PAR")
	return createFileStore(reqs)
}

func createKubeAPIServerStore(reqs Requires) (Provides, error) {
	k8sReqs := k8simpl.Requires{
		Config: reqs.Config,
		Log:    reqs.Log,
	}
	k8sProvides, err := k8simpl.NewComponent(k8sReqs)
	if err != nil {
		return Provides{}, err
	}
	return Provides{Comp: k8sProvides.Comp}, nil
}

func createFileStore(reqs Requires) (Provides, error) {
	fileReqs := fileimpl.Requires{
		Config: reqs.Config,
		Log:    reqs.Log,
	}
	fileProvides := fileimpl.NewComponent(fileReqs)
	return Provides{Comp: fileProvides.Comp}, nil
}
