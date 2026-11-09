// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	schemanoopimpl "github.com/DataDog/datadog-agent/comp/core/schema/noop-impl"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestSystemProbeSecretsResolveAndRefresh(t *testing.T) {
	config := mock.NewSystemProbe(t)
	file := filepath.Join(t.TempDir(), "system-probe.yaml")
	declaration := []byte("system_probe_config:\n  internal_profiling:\n    api_key: ENC[profile]")
	require.NoError(t, os.WriteFile(file, declaration, 0600))
	resolver := secretsmock.New(t)
	resolver.SetSecrets(map[string]string{"profile": "resolved-profile"})
	_, err := NewWithSecrets(file, "", &schemanoopimpl.SchemaNoop{}, resolver)
	require.NoError(t, err)
	require.Equal(t, "resolved-profile", config.GetString("system_probe_config.internal_profiling.api_key"))
	resolver.SetSecrets(map[string]string{"profile": "rotated-profile"})
	_, err = resolver.Resolve(declaration, "system-probe.yaml", "", "", true)
	require.NoError(t, err)
	require.Equal(t, "rotated-profile", config.GetString("system_probe_config.internal_profiling.api_key"))
}

func TestSystemProbeSecretFailureAbortsLoad(t *testing.T) {
	mock.NewSystemProbe(t)
	file := filepath.Join(t.TempDir(), "system-probe.yaml")
	require.NoError(t, os.WriteFile(file, []byte("system_probe_config:\n  internal_profiling:\n    api_key: ENC[missing]"), 0600))
	_, err := NewWithSecrets(file, "", &schemanoopimpl.SchemaNoop{}, secretsmock.New(t))
	require.ErrorContains(t, err, "missing")
}
