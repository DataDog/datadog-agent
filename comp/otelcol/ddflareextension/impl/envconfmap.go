// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ddflareextensionimpl

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/mohae/deepcopy"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
)

const schemeName = "env"

// envConfMap parses a yaml and provides methods to get the env variable names
type envConfMap struct {
	conf map[string]any
}

// newEnvConfMap creates a new envConfMap from the configProviderSettings.
// It replaces temporary the envProvider by a provider that replace the environment variable value with a UUID.
// This allows to easily identify which values were environment variables.
func newEnvConfMap(ctx context.Context, configProviderSettings otelcol.ConfigProviderSettings) (*envConfMap, error) {
	resolverSettings := configProviderSettings.ResolverSettings
	providerFactories := resolverSettings.ProviderFactories
	providersSettings := resolverSettings.ProviderSettings
	envProviderIndex := slices.IndexFunc(providerFactories, func(f confmap.ProviderFactory) bool {
		return f.Create(providersSettings).Scheme() == schemeName
	})
	if envProviderIndex == -1 {
		return nil, errors.New("env provider not found")
	}
	envProvider := providerFactories[envProviderIndex]
	uuids := make(map[string]string)

	// Temporary replace env provider by a provider that replaces the environment variable name with a UUID.
	providerFactories[envProviderIndex] = confmap.NewProviderFactory(newEnvToUUIDProvider(uuids))
	defer func() { providerFactories[envProviderIndex] = envProvider }()

	resolver, err := confmap.NewResolver(resolverSettings)
	if err != nil {
		return nil, err
	}

	cfg, err := resolver.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	conf := cfg.ToStringMap()

	// Remove all values which are not UUIDs (So keep only keys where the values were an env variable)
	visitMap(conf, func(v any) (any, bool) {
		if str, ok := v.(string); ok {
			value, found := uuids[str]
			return value, found
		}
		return nil, false
	})

	return &envConfMap{conf: conf}, nil

}

// useEnvVarNames replaces the values of the target map with the values of the source map
// For example replace REDACTED from confMap with ${env:DD_API_KEY}
func (e *envConfMap) useEnvVarNames(confMap map[string]any) map[string]any {
	mapReplaceValue(e.conf, confMap)
	return confMap
}

// useEnvVarValues replaces the env variable name with the values of the source map
// For example replace ${env:DD_API_KEY} with REDACTED from confMap
func (e *envConfMap) useEnvVarValues(confMap map[string]any) map[string]any {
	result := deepcopy.Copy(e.conf).(map[string]any)

	mapReplaceValue(confMap, result)
	return result
}

// mapReplaceValue replaces the values of the target map with the values of the source map
// if the key exists in both maps.
func mapReplaceValue(source map[string]any, target map[string]any) {
	for key, value := range source {
		if targetValue, ok := target[key]; ok {
			switch v := value.(type) {
			case map[string]any:
				targetValueMap, ok := targetValue.(map[string]any)
				if ok {
					mapReplaceValue(v, targetValueMap)
				}

			case []any:
				targetValueSlice, ok := targetValue.([]any)
				if ok {
					sliceReplaceIfExists(v, targetValueSlice)
				}
			default:
				target[key] = value
			}
		}
	}
}

func sliceReplaceIfExists(target []any, source []any) {
	for i, value := range source {
		if i < len(target) {
			switch v := value.(type) {
			case map[string]any:
				targetValueMap, ok := target[i].(map[string]any)
				if ok {
					mapReplaceValue(v, targetValueMap)
				}
			case []any:
				targetValue, ok := target[i].([]any)
				if ok {
					sliceReplaceIfExists(v, targetValue)
				}
			default:
				target[i] = value
			}
		}
	}
}

// visitMap visits all the elements of a map and applies the function f to each element.
// Empty map and empty slice are removed.
func visitMap(data map[string]any, f func(v any) (any, bool)) {
	for key, value := range data {
		switch v := value.(type) {
		case map[string]any:
			visitMap(v, f)
			if len(v) == 0 {
				delete(data, key)
			}
		case []any:
			values := visitSlice(v, f)
			if len(values) == 0 {
				delete(data, key)
			} else {
				data[key] = values
			}
		default:
			newValue, ok := f(value)
			if !ok {
				delete(data, key)
			} else {
				data[key] = newValue
			}
		}
	}
}

func visitSlice(data []any, f func(v any) (any, bool)) []any {
	var filtered []any
	for _, value := range data {
		switch v := value.(type) {
		case map[string]any:
			visitMap(v, f)
			if len(v) != 0 {
				filtered = append(filtered, v)
			}
		case []any:
			values := visitSlice(v, f)
			if len(values) != 0 {
				filtered = append(filtered, values)
			}
		default:
			newValue, ok := f(value)
			if ok {
				filtered = append(filtered, newValue)
			}
		}
	}
	return filtered
}

// envToUUIDProvider replaces the environment variable name with a UUID.
type envToUUIDProvider struct {
	uuidsToEnv map[string]string
}

func newEnvToUUIDProvider(uuids map[string]string) func(confmap.ProviderSettings) confmap.Provider {
	return func(confmap.ProviderSettings) confmap.Provider {
		return &envToUUIDProvider{uuidsToEnv: uuids}
	}
}

func (p *envToUUIDProvider) Retrieve(_ context.Context, uri string, _ confmap.WatcherFunc) (*confmap.Retrieved, error) {
	uuid := uuid.New().String()
	p.uuidsToEnv[uuid] = fmt.Sprintf("${%v}", uri)
	return confmap.NewRetrievedFromYAML([]byte(uuid))
}

func (p *envToUUIDProvider) Scheme() string {
	return schemeName
}

func (p *envToUUIDProvider) Shutdown(context.Context) error {
	return nil
}
