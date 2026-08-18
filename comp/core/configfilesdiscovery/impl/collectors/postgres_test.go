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

func TestIncludePostgresEnvVar(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		want    bool
	}{
		{name: "data directory", envName: "PGDATA", want: true},
		{name: "database name", envName: "POSTGRES_DB", want: true},
		{name: "user", envName: "POSTGRES_USER", want: true},
		{name: "host auth method", envName: "POSTGRES_HOST_AUTH_METHOD"},
		{name: "initdb waldir", envName: "POSTGRES_INITDB_WALDIR", want: true},
		{name: "unrelated", envName: "UNREQUESTED"},
		{name: "lowercase", envName: "postgres_db"},
		{name: "password", envName: "POSTGRES_PASSWORD"},
		{name: "password file", envName: "POSTGRES_PASSWORD_FILE"},
		{name: "initdb args", envName: "POSTGRES_INITDB_ARGS"},
		{name: "libpq password", envName: "PGPASSWORD"},
		{name: "libpq passfile", envName: "PGPASSFILE"},
		{name: "libpq service", envName: "PGSERVICE"},
		{name: "libpq servicefile", envName: "PGSERVICEFILE"},
		{name: "libpq options", envName: "PGOPTIONS"},
		{name: "libpq ssl key", envName: "PGSSLKEY"},
		{name: "libpq ssl cert", envName: "PGSSLCERT"},
		{name: "libpq ssl root cert", envName: "PGSSLROOTCERT"},
		{name: "libpq require ssl", envName: "PGREQUIRESSL"},
		{name: "database url", envName: "DATABASE_URL"},
		{name: "postgres url", envName: "POSTGRES_URL"},
		{name: "secret-shaped unrelated name", envName: "PGDATA_TOKEN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, includePostgresEnvVar(tt.envName))
		})
	}
}

func TestPostgresCollectorCollectsEnvVars(t *testing.T) {
	reader := &postgresCollectorTestReader{
		env: map[string]string{
			"POSTGRES_USER": "app",
			"PGDATA":        "/var/lib/postgresql/data",
			"POSTGRES_DB":   "app",
			"UNREQUESTED":   "value",
		},
	}
	collector := NewPostgres()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "PGDATA", Value: "/var/lib/postgresql/data"},
		{Name: "POSTGRES_DB", Value: "app"},
		{Name: "POSTGRES_USER", Value: "app"},
	}, collected.EnvVars)
	assert.Empty(t, collected.ConfigFiles)
}

func TestPostgresCollectorSkipsWhenNoSelectedEnvVars(t *testing.T) {
	reader := &postgresCollectorTestReader{
		env: map[string]string{
			"UNREQUESTED": "value",
		},
	}

	collected, err := NewPostgres().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestPostgresCollectorReturnsEnvVarErrors(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	reader := &postgresCollectorTestReader{readEnvVarsErr: expectedErr}
	collector := NewPostgres()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestPostgresCollectorCanCollectFromProcessAlwaysFalse(t *testing.T) {
	collector := postgresConfigCollector{}
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{}))
}

type postgresCollectorTestReader struct {
	env            map[string]string
	readEnvVarsErr error
}

func (r *postgresCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *postgresCollectorTestReader) Close() {}

func (r *postgresCollectorTestReader) ReadFile(context.Context, string) (configfilesdiscoveryimpl.ConfigFile, error) {
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("not implemented")
}

func (r *postgresCollectorTestReader) ReadEnvVars(_ context.Context, predicate configfilesdiscoveryimpl.ConfigEnvVarPredicate) (map[string]string, error) {
	if r.readEnvVarsErr != nil {
		return nil, r.readEnvVarsErr
	}

	env := make(map[string]string)
	for name, value := range r.env {
		if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
			continue
		}
		if predicate != nil && !predicate(name) {
			continue
		}
		env[name] = value
	}
	return env, nil
}

func (r *postgresCollectorTestReader) ReadRuntimeCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	return configfilesdiscoveryimpl.TargetCommandline{}, nil
}

func (r *postgresCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	return nil
}
