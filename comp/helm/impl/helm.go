// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package helmimpl implements the helm component.
package helmimpl

import (
	"context"
	"fmt"

	"helm.sh/helm/v4/pkg/action"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	helm "github.com/DataDog/datadog-agent/comp/helm/def"
)

// Requires defines the dependencies for the helm component.
type Requires struct {
	Log log.Component
}

// Provides defines the output of the helm component.
type Provides struct {
	Comp helm.Component
}

type helmComp struct {
	log log.Component
}

// NewComponent creates a new helm component.
func NewComponent(reqs Requires) Provides {
	return Provides{
		Comp: &helmComp{log: reqs.Log},
	}
}

// Rollback rolls back the named release in the given namespace to a prior
// revision. A revision of 0 rolls back to the immediately previous release.
func (h *helmComp) Rollback(_ context.Context, releaseName, namespace string, revision int) error {
	if releaseName == "" {
		return fmt.Errorf("release name is required")
	}
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if revision < 0 {
		return fmt.Errorf("revision must be >= 0")
	}

	getter := newInClusterRESTClientGetter(namespace)

	cfg := action.NewConfiguration()
	if err := cfg.Init(getter, namespace, ""); err != nil {
		return fmt.Errorf("init helm action: %w", err)
	}

	client := action.NewRollback(cfg)
	client.Version = revision

	if err := client.Run(releaseName); err != nil {
		return fmt.Errorf("rollback %q in %q: %w", releaseName, namespace, err)
	}
	h.log.Infof("Rolled back release %q in namespace %q to revision %d", releaseName, namespace, revision)
	return nil
}
