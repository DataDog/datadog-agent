// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
)

// envReferencePrefix starts the environment references that the configurations of the flare keep
const envReferencePrefix = "${env:"

// envReferences keeps the environment references of the configurations that the flare reads from
// the files, when the collector can't be asked. Those files don't tell which settings are secrets,
// and the value of a variable can be one that no scrubber recognizes, like the one of a header. Like
// in the live flare, the configurations have the references, not the values.
//
// The resolver expands again a reference that a provider gives as the value. Like the extension of
// the collector, the env provider gives a token instead, which restore replaces by the reference once
// the configuration is resolved.
//
// The locations of the configuration that use the env provider, like --config env:NAME, are not
// references: the variable holds the configuration, which the env provider reads as usual.
type envReferences struct {
	mu        sync.Mutex
	refs      []string
	locations map[string]struct{}
}

// newEnvReferences returns the references of the resolver that loads the configuration locations uris
func newEnvReferences(uris []string) *envReferences {
	locations := make(map[string]struct{}, len(uris))
	for _, uri := range uris {
		locations[uri] = struct{}{}
	}
	return &envReferences{locations: locations}
}

// providerFactory returns the env provider that gives the tokens
func (e *envReferences) providerFactory() confmap.ProviderFactory {
	return confmap.NewProviderFactory(func(settings confmap.ProviderSettings) confmap.Provider {
		return envReferenceProvider{references: e, env: envprovider.NewFactory().Create(settings)}
	})
}

// token returns the token that stands for the reference ${uri}, where uri is env:NAME
func (e *envReferences) token(uri string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.refs = append(e.refs, "${"+uri+"}")
	return fmt.Sprintf("<<env-reference-%d>>", len(e.refs)-1)
}

// restore replaces the tokens of a resolved configuration by the references they stand for
func (e *envReferences) restore(confMap map[string]any) {
	e.mu.Lock()
	oldNew := make([]string, 0, 2*len(e.refs))
	for i, ref := range e.refs {
		oldNew = append(oldNew, fmt.Sprintf("<<env-reference-%d>>", i), ref)
	}
	e.mu.Unlock()
	replacer := strings.NewReplacer(oldNew...)
	mapStrings(confMap, replacer.Replace)
}

// envReferenceProvider gives a token for ${env:NAME}, and the configuration that a location holds
type envReferenceProvider struct {
	references *envReferences
	env        confmap.Provider
}

func (p envReferenceProvider) Retrieve(ctx context.Context, uri string, watcher confmap.WatcherFunc) (*confmap.Retrieved, error) {
	if _, ok := p.references.locations[uri]; ok {
		return p.env.Retrieve(ctx, uri, watcher)
	}
	return confmap.NewRetrieved(p.references.token(uri))
}

func (envReferenceProvider) Scheme() string {
	return "env"
}

func (envReferenceProvider) Shutdown(context.Context) error {
	return nil
}

// mapStrings applies f to the strings of a configuration, in place
func mapStrings(value any, f func(string) string) any {
	switch v := value.(type) {
	case string:
		return f(v)
	case map[string]any:
		for key, item := range v {
			v[key] = mapStrings(item, f)
		}
	case []any:
		for i, item := range v {
			v[i] = mapStrings(item, f)
		}
	}
	return value
}

// envReferencesOnly returns the settings of a configuration whose value has an environment reference,
// like the env configuration of the live flare
func envReferencesOnly(value any) any {
	switch v := value.(type) {
	case string:
		if strings.Contains(v, envReferencePrefix) {
			return v
		}
	case map[string]any:
		kept := map[string]any{}
		for key, item := range v {
			if filtered := envReferencesOnly(item); filtered != nil {
				kept[key] = filtered
			}
		}
		if len(kept) > 0 {
			return kept
		}
	case []any:
		var kept []any
		for _, item := range v {
			if filtered := envReferencesOnly(item); filtered != nil {
				kept = append(kept, filtered)
			}
		}
		if len(kept) > 0 {
			return kept
		}
	}
	return nil
}
