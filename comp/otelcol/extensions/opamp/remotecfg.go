// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package opamp

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/confmap"
)

const remoteConfigScheme = "opampcfg"

// RemoteConfigURI is the fixed URI the collector resolves to get the current
// remote config overlay.  Adding it last in the URI list means its values
// override earlier (file-based) settings when they are merged.
const RemoteConfigURI = "opampcfg:remote"

// RemoteConfigProvider is a confmap.Provider that serves the YAML config
// currently pushed by the OpAMP server.  Push() updates the content and
// triggers a collector hot-reload via the WatcherFunc registered during
// Retrieve().
//
// speky:DDOT#OTELCOL030 speky:DDOT#OTELCOL031 speky:DDOT#OTELCOL036
type RemoteConfigProvider struct {
	mu      sync.Mutex
	content []byte
	watcher confmap.WatcherFunc
}

// Scheme returns the URI scheme ("opampcfg") for this provider.
func (p *RemoteConfigProvider) Scheme() string { return remoteConfigScheme }

// Retrieve is called by the confmap resolver at startup and after each change
// event.  It stores watcher for later use by Push() and returns the current
// remote config (or an empty map when nothing has been pushed yet).
func (p *RemoteConfigProvider) Retrieve(_ context.Context, _ string, watcher confmap.WatcherFunc) (*confmap.Retrieved, error) {
	p.mu.Lock()
	p.watcher = watcher
	content := p.content
	p.mu.Unlock()
	if len(content) == 0 {
		return confmap.NewRetrievedFromYAML([]byte("{}"))
	}
	return confmap.NewRetrievedFromYAML(content)
}

// Shutdown is a no-op; the provider is owned by the collector process.
func (p *RemoteConfigProvider) Shutdown(_ context.Context) error { return nil }

// Push stores yamlContent as the new remote config overlay and signals the
// collector to reload its pipeline configuration.
func (p *RemoteConfigProvider) Push(yamlContent []byte) {
	p.mu.Lock()
	p.content = yamlContent
	w := p.watcher
	p.mu.Unlock()
	if w != nil {
		w(&confmap.ChangeEvent{})
	}
}

// NewRemoteConfigProviderFactory wraps the shared provider in a
// confmap.ProviderFactory so it can be registered in ConfigProviderSettings.
func NewRemoteConfigProviderFactory(p *RemoteConfigProvider) confmap.ProviderFactory {
	return confmap.NewProviderFactory(func(confmap.ProviderSettings) confmap.Provider {
		return p
	})
}
