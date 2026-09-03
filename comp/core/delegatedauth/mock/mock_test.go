// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package mock

import (
	"context"
	"fmt"
	"sync"
	"testing"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddInstanceReplacesProviderRegistration(t *testing.T) {
	m := &Mock{
		ProviderForInstanceFunc: func(params delegatedauth.InstanceParams) delegatedauth.Provider {
			return StaticProvider{Key: params.Directive}
		},
	}
	const instanceKey = "additional_endpoints[org-a]"

	_, err := m.AddInstance(context.Background(), delegatedauth.InstanceParams{
		APIKeyConfigKey: instanceKey,
		ConfigKey:       "additional_endpoints",
		Destination:     "https://old.datadoghq.com",
		Directive:       "DELA(old-org, aws)",
	})
	require.NoError(t, err)

	_, err = m.AddInstance(context.Background(), delegatedauth.InstanceParams{
		APIKeyConfigKey: instanceKey,
		ConfigKey:       "additional_endpoints",
		Destination:     "https://new.datadoghq.com",
		Directive:       "DELA(new-org, aws)",
	})
	require.NoError(t, err)

	assert.Empty(t, m.ProvidersFor("additional_endpoints", "https://old.datadoghq.com"))
	assert.Nil(t, m.ProviderForDirective("additional_endpoints", "https://old.datadoghq.com", "DELA(old-org, aws)"))
	assert.Len(t, m.ProvidersFor("additional_endpoints", "https://new.datadoghq.com"), 1)
	assert.NotNil(t, m.ProviderForDirective("additional_endpoints", "https://new.datadoghq.com", "DELA(new-org, aws)"))
}

func TestProvidersForReturnsCopy(t *testing.T) {
	m := &Mock{}
	_, err := m.AddInstance(context.Background(), delegatedauth.InstanceParams{
		APIKeyConfigKey: "api_key",
		ConfigKey:       "api_key",
	})
	require.NoError(t, err)

	providers := m.ProvidersFor("api_key", "")
	require.Len(t, providers, 1)
	providers[0] = nil

	assert.NotNil(t, m.ProvidersFor("api_key", "")[0])
}

func TestConcurrentRegistrationAndLookup(t *testing.T) {
	m := &Mock{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			directive := fmt.Sprintf("DELA(org-%d, aws)", i)
			_, err := m.AddInstance(context.Background(), delegatedauth.InstanceParams{
				APIKeyConfigKey: fmt.Sprintf("instance-%d", i),
				ConfigKey:       "additional_endpoints",
				Destination:     "https://app.datadoghq.com",
				Directive:       directive,
			})
			assert.NoError(t, err)
			m.ProvidersFor("additional_endpoints", "https://app.datadoghq.com")
			m.ProviderForDirective("additional_endpoints", "https://app.datadoghq.com", directive)
		}(i)
	}
	wg.Wait()

	assert.Len(t, m.ProvidersFor("additional_endpoints", "https://app.datadoghq.com"), 50)
}
