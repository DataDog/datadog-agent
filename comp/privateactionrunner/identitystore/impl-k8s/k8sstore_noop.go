// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

package k8sstoreimpl

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	identitystore "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/def"
)

// Requires defines the dependencies for the K8s-based identity store
type Requires struct {
	compdef.In

	Config config.Component
	Log    log.Component
}

// Provides defines the output of the K8s-based identity store
type Provides struct {
	compdef.Out

	Comp identitystore.Component
}

type noopStore struct{}

// NewComponent creates a noop K8s identity store when kubeapiserver build tag is not set
func NewComponent(reqs Requires) (Provides, error) {
	return Provides{}, errors.New("Kubernetes identity store is not available in this build (kubeapiserver build tag required)")
}

func (n *noopStore) GetIdentity(ctx context.Context) (*identitystore.Identity, error) {
	return nil, errors.New("Kubernetes identity store is not available in this build")
}

func (n *noopStore) PersistIdentity(ctx context.Context, identity *identitystore.Identity) error {
	return errors.New("Kubernetes identity store is not available in this build")
}

func (n *noopStore) DeleteIdentity(ctx context.Context) error {
	return errors.New("Kubernetes identity store is not available in this build")
}
