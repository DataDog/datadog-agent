// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connectionsforwarderimpl

import (
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
	t.Helper()
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

func TestCreateParamsSameHostEndpointsKeepAllEntriesAfterUpdate(t *testing.T) {
	const domain = "https://process.second-org.datadoghq.com"
	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "main-key")
	cfg.SetInTest("process_config.additional_endpoints", map[string][]string{
		domain:       {"bare-key"},
		domain + "/": {"slash-key"},
	})
	r := newResolver(t, cfg, domain)
	require.ElementsMatch(t, []string{"bare-key", "slash-key"}, r.GetAPIKeys())

	cfg.Set("process_config.additional_endpoints", map[string][]string{
		domain:       {"bare-key2"},
		domain + "/": {"slash-key2"},
	}, model.SourceSecret)

	assert.ElementsMatch(t, []string{"bare-key2", "slash-key2"}, r.GetAPIKeys())
}

func TestCreateParamsSameHostDelegatedAuthAliases(t *testing.T) {
	const domain = "https://process.second-org.datadoghq.com"
	const setting = "process_config.additional_endpoints"
	for _, suffixes := range [][2]string{{"", "/"}, {"/", "/v1"}} {
		for _, pendingOnly := range []bool{false, true} {
			name := suffixes[0] + "+" + suffixes[1]
			if pendingOnly {
				name += "/pending-only"
			}
			t.Run(name, func(t *testing.T) {
				first, second := domain+suffixes[0], domain+suffixes[1]
				cfg := config.NewMock(t)
				cfg.SetInTest("api_key", "main-key")
				var mainKeys []string
				firstKeys := []string{"static-key"}
				if pendingOnly {
					firstKeys = []string{"DELA[first-org, aws]"}
				} else {
					cfg.SetInTest("process_config.process_dd_url", domain)
					mainKeys = []string{"main-key"}
				}
				cfg.SetInTest(setting, map[string][]string{
					first: firstKeys, second: {"DELA[second-org, aws]"},
				})
				r := newResolver(t, cfg, domain)

				apiKeys, _ := r.GetAPIKeysInfo()
				var pending bool
				for _, keys := range apiKeys {
					pending = pending || keys.HasPendingDelegatedAuth
				}
				require.True(t, pending, "startup reconciliation must retain pending aliases")
				require.True(t, r.IsUsable())
				if pendingOnly {
					assert.Empty(t, r.GetAPIKeys())
				} else {
					assert.ElementsMatch(t, []string{"main-key", "static-key"}, r.GetAPIKeys())
				}

				cfg.Set(setting, map[string][]string{
					first: {"first-key"}, second: {"second-key"},
				}, model.SourceSecret)
				assert.ElementsMatch(t, append(mainKeys, "first-key", "second-key"), r.GetAPIKeys())

				cfg.Set(setting, map[string][]string{
					first: {"first-rotated"}, second: {"second-rotated"},
				}, model.SourceSecret)
				assert.ElementsMatch(t, append(mainKeys, "first-rotated", "second-rotated"), r.GetAPIKeys())

				cfg.Set(setting, map[string][]string{second: {"second-rotated"}}, model.SourceSecret)
				assert.ElementsMatch(t, append(mainKeys, "second-rotated"), r.GetAPIKeys())

				cfg.Set(setting, map[string][]string{
					first: {"first-restored"}, second: {"second-rotated"},
				}, model.SourceSecret)
				assert.ElementsMatch(t, append(mainKeys, "first-restored", "second-rotated"), r.GetAPIKeys())
			})
		}
	}
}
