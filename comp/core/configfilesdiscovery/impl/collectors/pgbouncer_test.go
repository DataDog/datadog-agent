// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"errors"
	"testing"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncludePgbouncerEnvVar(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"PGBOUNCER_PORT", true},
		{"PGBOUNCER_DEFAULT_POOL_SIZE", true},
		{"PGBOUNCER_AUTH_TYPE", true},
		{"AUTH_TYPE", true},
		{"POOL_MODE", true},
		{"MAX_CLIENT_CONN", true},
		{"DB_HOST", true},
		{"PGBOUNCER_DSN_0", false},
		{"PGBOUNCER_DATABASE", false},
		{"PGBOUNCER_PASSWORD", false},
		{"DATABASE_URL", false},
		{"DATABASE_URLS", false},
		{"DB_PASSWORD", false},
		{"PGBOUNCER_AUTH_QUERY", false},
		{"SERVER_RESET_QUERY", false},
		{"PGBOUNCER_EXTRA_FLAGS", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, includePgbouncerEnvVar(tt.name))
		})
	}
}

func TestPgbouncerCollectorCollectsDogfoodEnvVars(t *testing.T) {
	reader := &pgbouncerCollectorTestReader{env: map[string]string{
		"AUTH_TYPE":          "md5",
		"POOL_MODE":          "transaction",
		"MAX_CLIENT_CONN":    "100",
		"DB_HOST":            "postgres",
		"DB_NAME":            "app",
		"DB_USER":            "pgbouncer",
		"DATABASE_URL":       "postgres://user:password@postgres/app",
		"DB_PASSWORD":        "do-not-send",
		"SERVER_RESET_QUERY": "DISCARD ALL",
	}}

	collected, err := NewPgbouncer().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "AUTH_TYPE", Value: "md5"},
		{Name: "DB_HOST", Value: "postgres"},
		{Name: "DB_NAME", Value: "app"},
		{Name: "DB_USER", Value: "pgbouncer"},
		{Name: "MAX_CLIENT_CONN", Value: "100"},
		{Name: "POOL_MODE", Value: "transaction"},
	}, collected.EnvVars)
	assert.Empty(t, collected.ConfigFiles)
}

func TestPgbouncerCollectorDoesNotUseProcessCommandline(t *testing.T) {
	assert.False(t, NewPgbouncer().CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"pgbouncer", "/etc/pgbouncer/pgbouncer.ini"}}))
}

func TestPgbouncerCollectorReturnsEnvReadError(t *testing.T) {
	expectedErr := errors.New("environment unavailable")
	_, err := NewPgbouncer().Collect(context.Background(), &pgbouncerCollectorTestReader{readEnvVarsErr: expectedErr})
	require.ErrorIs(t, err, expectedErr)
}

type pgbouncerCollectorTestReader struct {
	env            map[string]string
	readEnvVarsErr error
}

func (r *pgbouncerCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *pgbouncerCollectorTestReader) Close() {}

func (r *pgbouncerCollectorTestReader) ReadFile(context.Context, string) (configfilesdiscoveryimpl.ConfigFile, error) {
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("not implemented")
}

func (r *pgbouncerCollectorTestReader) ReadEnvVars(_ context.Context, predicate configfilesdiscoveryimpl.ConfigEnvVarPredicate) (map[string]string, error) {
	if r.readEnvVarsErr != nil {
		return nil, r.readEnvVarsErr
	}
	env := make(map[string]string)
	for name, value := range r.env {
		if predicate(name) {
			env[name] = value
		}
	}
	return env, nil
}

func (r *pgbouncerCollectorTestReader) ReadRuntimeCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	return configfilesdiscoveryimpl.TargetCommandline{}, errors.New("not implemented")
}

func (r *pgbouncerCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	return nil
}
