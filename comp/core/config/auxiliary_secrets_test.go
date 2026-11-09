// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	delegatedauthnooptypes "github.com/DataDog/datadog-agent/comp/core/delegatedauth/noop-impl/types"
	schemanoopimpl "github.com/DataDog/datadog-agent/comp/core/schema/noop-impl"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestSecurityAgentSecretsResolvedAfterMerge(t *testing.T) {
	for _, streamed := range []bool{false, true} {
		name := "disk"
		if streamed {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			config := NewMock(t)
			dir := t.TempDir()
			mainFile := filepath.Join(dir, "datadog.yaml")
			securityFile := filepath.Join(dir, "security-agent.yaml")
			main := "secret_backend_command: mock\nsecurity_agent:\n  internal_profiling:\n    api_key: ENC[overridden]\n"
			require.NoError(t, os.WriteFile(mainFile, []byte(main), 0600))
			require.NoError(t, os.WriteFile(securityFile, []byte("security_agent:\n  internal_profiling:\n    api_key: ENC[security]\n"), 0600))
			resolver := secretsmock.New(t)
			// The overridden main-file handle must never be fetched.
			resolver.SetSecrets(map[string]string{"security": "resolved-security"})
			deps := dependencies{
				Params:        NewSecurityAgentParams([]string{mainFile, securityFile}),
				Secret:        resolver,
				Schema:        &schemanoopimpl.SchemaNoop{},
				DelegatedAuth: &delegatedauthnooptypes.DelegatedAuthNoop{},
			}
			origin := "datadog.yaml"
			if streamed {
				config.Set("secret_backend_command", "mock", model.SourceFile)
				config.Set("security_agent.internal_profiling.api_key", "ENC[overridden]", model.SourceFile)
				config.Set("security_agent.internal_profiling.api_key", "streamed-main-secret", model.SourceSecret)
				deps.Cfgstream = activeConfigStream{}
				origin = "security-agent.yaml"
			}
			cfg, err := newConfig(deps)
			require.NoError(t, err)
			require.Equal(t, "resolved-security", cfg.GetString("security_agent.internal_profiling.api_key"))
			resolver.SetSecrets(map[string]string{"security": "rotated-security"})
			_, err = resolver.Resolve([]byte("security_agent:\n  internal_profiling:\n    api_key: ENC[security]"), origin, "", "", true)
			require.NoError(t, err)
			require.Equal(t, "rotated-security", cfg.GetString("security_agent.internal_profiling.api_key"))
		})
	}
}

func TestSecurityAgentSecretFailureAbortsLoad(t *testing.T) {
	NewMock(t)
	dir := t.TempDir()
	mainFile, securityFile := filepath.Join(dir, "datadog.yaml"), filepath.Join(dir, "security-agent.yaml")
	require.NoError(t, os.WriteFile(mainFile, []byte("secret_backend_command: mock"), 0600))
	require.NoError(t, os.WriteFile(securityFile, []byte("security_agent:\n  internal_profiling:\n    api_key: ENC[missing]"), 0600))
	_, err := newConfig(dependencies{
		Params: NewSecurityAgentParams([]string{mainFile, securityFile}),
		Secret: secretsmock.New(t), Schema: &schemanoopimpl.SchemaNoop{}, DelegatedAuth: &delegatedauthnooptypes.DelegatedAuthNoop{},
	})
	require.ErrorContains(t, err, "missing")
}
