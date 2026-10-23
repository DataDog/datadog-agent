// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connectionsforwarderimpl

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
)

// newResolver builds the connections forwarder's resolvers from cfg, which also hooks them
// to config updates, and returns the one for domain.
func newResolver(t *testing.T, cfg config.Component, domain string) resolver.DomainResolver {
	eps, err := endpoint.GetAPIEndpoints(cfg)
	require.NoError(t, err)
	opts, err := createParams(cfg, logmock.New(t), 1, eps)
	require.NoError(t, err)
	r := opts.DomainResolvers[domain]
	require.NotNil(t, r)
	return r
}

func TestCreateParamsDeliversKeyUpdatesForPathBearingEndpoint(t *testing.T) {
	// The resolver is named scheme://host, but updates land under the key as written.
	const configKey = "https://process.second-org.datadoghq.com/"
	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "main-key")
	cfg.SetInTest("process_config.additional_endpoints", map[string][]string{configKey: {"old-key"}})
	r := newResolver(t, cfg, "https://process.second-org.datadoghq.com")

	cfg.Set("process_config.additional_endpoints", map[string][]string{configKey: {"new-key"}}, model.SourceSecret)

	assert.Equal(t, []string{"new-key"}, r.GetAPIKeys())
}

func TestCreateParamsSameHostEndpointsKeepOnlyExactEntryAfterUpdate(t *testing.T) {
	// Two entries for one host share a resolver that can only track one config key: after an
	// update only the entry written as the domain keeps its keys, until restart.
	const domain = "https://process.second-org.datadoghq.com"
	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "main-key")
	cfg.SetInTest("process_config.additional_endpoints", map[string][]string{
		domain:       {"bare-key"},
		domain + "/": {"slash-key"},
	})
	r := newResolver(t, cfg, domain)
	keys := r.GetAPIKeys()
	sort.Strings(keys)
	require.Equal(t, []string{"bare-key", "slash-key"}, keys)

	cfg.Set("process_config.additional_endpoints", map[string][]string{
		domain:       {"bare-key2"},
		domain + "/": {"slash-key2"},
	}, model.SourceSecret)

	assert.Equal(t, []string{"bare-key2"}, r.GetAPIKeys())
}
