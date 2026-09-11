// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact/catalog"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const authoredScriptPackagePrefix = "com.datadoghq.authoredscripts."

// ErrCatalogNotReady is returned before Remote Config has delivered the first
// valid catalog snapshot.
var ErrCatalogNotReady = errors.New("authored-script catalog is not ready")

// RemoteConfigCatalog adapts the shared immutable catalog to authored-script
// FQN lookups. It does not change or replace Fleet's Remote Config subscription.
type RemoteConfigCatalog struct {
	client rcclient.Client
	live   catalog.LiveCatalog
}

// NewRemoteConfigCatalog creates an authored-script catalog backed by client.
func NewRemoteConfigCatalog(client rcclient.Client) *RemoteConfigCatalog {
	return &RemoteConfigCatalog{client: client}
}

// Start subscribes this opt-in consumer to UPDATER_CATALOG_DD.
func (c *RemoteConfigCatalog) Start() error {
	if c == nil || c.client == nil {
		return errors.New("remote config client is required for authored-script catalog")
	}
	c.client.Subscribe(state.ProductUpdaterCatalogDD, c.onUpdate)
	return nil
}

func (c *RemoteConfigCatalog) Lookup(fqn string) (Descriptor, error) {
	if c == nil {
		return Descriptor{}, ErrCatalogNotReady
	}
	if !strings.HasPrefix(fqn, authoredScriptPackagePrefix) {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrPackageNotConfigured, fqn)
	}
	value, err := c.live.Select(catalog.Selector{
		Package:      strings.ToLower(fqn),
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	})
	if errors.Is(err, catalog.ErrNotReady) {
		return Descriptor{}, ErrCatalogNotReady
	}
	if errors.Is(err, catalog.ErrNotFound) {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrPackageNotConfigured, fqn)
	}
	if err != nil {
		return Descriptor{}, err
	}
	return descriptorFromArtifact(value), nil
}

// WithAuthorized atomically verifies that descriptor remains in the current
// catalog while use performs the authorization-sensitive operation.
func (c *RemoteConfigCatalog) WithAuthorized(descriptor Descriptor, use func() error) error {
	sharedDescriptor, err := descriptor.catalogDescriptor()
	if err != nil {
		return err
	}
	return c.live.WithAuthorized(sharedDescriptor, use)
}

func (c *RemoteConfigCatalog) onUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	documents := make(map[string][]byte, len(updates))
	for configPath, update := range updates {
		documents[configPath] = update.Config
	}
	snapshot, err := catalog.ParseMap(documents)
	if err == nil {
		err = c.live.Replace(snapshot)
	}
	if err != nil {
		for configPath := range updates {
			applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		}
		return
	}
	for configPath := range updates {
		applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}
