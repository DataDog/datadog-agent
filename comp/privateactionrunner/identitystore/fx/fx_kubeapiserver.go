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
	filestoreimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/impl-file"
	k8sstoreimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/impl-k8s"
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
	// Determine which implementation to use based on config
	useK8sSecret := reqs.Config.GetBool("privateactionrunner.use_k8s_secret")

	if useK8sSecret {
		reqs.Log.Info("Using Kubernetes secret-based identity store for PAR")

		// Create K8s implementation
		k8sReqs := k8sstoreimpl.Requires{
			Config: reqs.Config,
			Log:    reqs.Log,
		}
		k8sProvides, err := k8sstoreimpl.NewComponent(k8sReqs)
		if err != nil {
			// Fall back to file-based if K8s client fails
			reqs.Log.Warnf("Failed to create K8s identity store, falling back to file-based: %v", err)
			return Provides{Comp: createFileStore(reqs)}, nil
		}
		return Provides{Comp: k8sProvides.Comp}, nil
	}

	reqs.Log.Info("Using file-based identity store for PAR")
	return Provides{Comp: createFileStore(reqs)}, nil
}

func createFileStore(reqs Requires) identitystore.Component {
	fileReqs := filestoreimpl.Requires{
		Config: reqs.Config,
		Log:    reqs.Log,
	}
	return filestoreimpl.NewComponent(fileReqs).Comp
}
