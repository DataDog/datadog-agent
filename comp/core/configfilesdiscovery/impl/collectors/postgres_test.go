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

func TestPostgresGetConfigPath(t *testing.T) {
	tests := []struct {
		name        string
		commandline configfilesdiscoveryimpl.TargetCommandline
		wantPath    string
		wantOK      bool
	}{
		{
			name: "official image bare startup",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"postgres"},
			},
		},
		{
			name: "shell form",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"/bin/sh", "-c", "postgres -c config_file=/etc/postgresql/postgresql.conf"},
			},
			wantPath: "/etc/postgresql/postgresql.conf",
			wantOK:   true,
		},
		{
			name: "explicit config_file short option",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"},
			},
			wantPath: "/etc/postgresql/postgresql.conf",
			wantOK:   true,
		},
		{
			name: "explicit config_file long option",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"postgres", "--config_file=/etc/postgresql/postgresql.conf"},
			},
			wantPath: "/etc/postgresql/postgresql.conf",
			wantOK:   true,
		},
		{
			name: "explicit data directory",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"postgres", "-D", "/var/lib/postgresql/data"},
			},
			wantPath: "/var/lib/postgresql/data/postgresql.conf",
			wantOK:   true,
		},
		{
			name: "config_file wins over data directory",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"postgres", "-D", "/var/lib/postgresql/data", "-c", "config_file=/etc/postgresql/postgresql.conf"},
			},
			wantPath: "/etc/postgresql/postgresql.conf",
			wantOK:   true,
		},
		{
			name: "relative config_file resolved with working dir",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args:       []string{"postgres", "-c", "config_file=postgresql.conf"},
				WorkingDir: "/etc/postgresql",
			},
			wantPath: "/etc/postgresql/postgresql.conf",
			wantOK:   true,
		},
		{
			name: "relative data directory resolved with working dir",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args:       []string{"postgres", "-D", "data"},
				WorkingDir: "/var/lib/postgresql",
			},
			wantPath: "/var/lib/postgresql/data/postgresql.conf",
			wantOK:   true,
		},
		{
			name: "relative config_file without absolute working dir",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"postgres", "-c", "config_file=postgresql.conf"},
			},
		},
		{
			name: "empty explicit config_file value",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"postgres", "-c", "config_file="},
			},
		},
		{
			name: "postgres not present",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"pg_ctl", "start"},
			},
		},
		{
			name: "flags only, no explicit source",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"postgres", "-p", "5432"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configArg, gotOK := postgresGetConfigArgFromCommandline(tt.commandline.Args)
			var gotPath string
			if gotOK {
				gotPath, gotOK = resolveConfigPath(configArg, tt.commandline.WorkingDir)
			}

			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}

func TestIncludePostgresEnvVar(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		want    bool
	}{
		{name: "data directory", envName: "PGDATA", want: true},
		{name: "database name", envName: "POSTGRES_DB", want: true},
		{name: "user", envName: "POSTGRES_USER", want: true},
		{name: "host auth method", envName: "POSTGRES_HOST_AUTH_METHOD", want: true},
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

func TestPostgresCollectorResolvesAndReadsRelativeProcessConfig(t *testing.T) {
	eventArgs := []string{"postgres", "-D", "data"}
	eventCommandline := configfilesdiscoveryimpl.TargetCommandline{
		Args:       eventArgs,
		WorkingDir: "/var/lib/postgresql",
	}
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/docker-entrypoint.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{eventCommandline},
		file:                    configfilesdiscoveryimpl.ConfigFile{Path: "/var/lib/postgresql/data/postgresql.conf"},
	}

	collector := postgresConfigCollector{}
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: eventArgs}))
	assert.True(t, collector.CanCollectFromProcess(eventCommandline))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"},
	}))

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/var/lib/postgresql/data/postgresql.conf"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestPostgresCollectorReadsDetectedConfig(t *testing.T) {
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/etc/postgresql/postgresql.conf",
			Content: []byte("port = 5432\n"),
		},
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
	assert.Equal(t, []string{"/etc/postgresql/postgresql.conf"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configfilesdiscoveryimpl.ConfigFile{
		Path:          "/etc/postgresql/postgresql.conf",
		Content:       []byte("port = 5432\n"),
		PayloadFormat: postgresConfigPayloadFormat,
	}, collected.ConfigFiles[0])
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "PGDATA", Value: "/var/lib/postgresql/data"},
		{Name: "POSTGRES_DB", Value: "app"},
		{Name: "POSTGRES_USER", Value: "app"},
	}, collected.EnvVars)
}

func TestPostgresCollectorCollectsEnvOnly(t *testing.T) {
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"postgres"},
		},
		env: map[string]string{
			"POSTGRES_DB": "app",
		},
	}
	collector := NewPostgres()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "POSTGRES_DB", Value: "app"},
	}, collected.EnvVars)
}

func TestPostgresCollectorSkipsWhenNoConfigAndNoEnv(t *testing.T) {
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"postgres"},
		},
	}

	collected, err := NewPostgres().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestPostgresCollectorUsesPGDataFallback(t *testing.T) {
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"postgres"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/var/lib/postgresql/data/postgresql.conf",
			Content: []byte("port = 5432\n"),
		},
		env: map[string]string{
			"PGDATA": "/var/lib/postgresql/data",
		},
	}

	collected, err := NewPostgres().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/var/lib/postgresql/data/postgresql.conf"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, postgresConfigPayloadFormat, collected.ConfigFiles[0].PayloadFormat)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "PGDATA", Value: "/var/lib/postgresql/data"},
	}, collected.EnvVars)
}

func TestPostgresCollectorBlocksPGDataFallbackWhenArgvSourceUnusable(t *testing.T) {
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"postgres", "-c", "config_file="},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path: "/var/lib/postgresql/data/postgresql.conf",
		},
		env: map[string]string{
			"PGDATA": "/var/lib/postgresql/data",
		},
	}

	collected, err := NewPostgres().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "PGDATA", Value: "/var/lib/postgresql/data"},
	}, collected.EnvVars)
}

func TestPostgresCollectorSkipsConflictingProcessConfigPaths(t *testing.T) {
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/docker-entrypoint.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"postgres", "-D", "/data1"}},
			{Args: []string{"postgres", "-D", "/data2"}},
		},
	}

	collected, err := NewPostgres().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Empty(t, collected.EnvVars)
}

func TestPostgresCollectorReturnsCommandlineErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &postgresCollectorTestReader{commandlineErr: expectedErr}
	collector := NewPostgres()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestPostgresCollectorReadsDetectedConfigWhenEnvVarsFail(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/etc/postgresql/postgresql.conf",
			Content: []byte("port = 5432\n"),
		},
		readEnvVarsErr: expectedErr,
	}
	collector := NewPostgres()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/postgresql/postgresql.conf"}, reader.readFileCalls)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigFile{
		{
			Path:          "/etc/postgresql/postgresql.conf",
			Content:       []byte("port = 5432\n"),
			PayloadFormat: postgresConfigPayloadFormat,
		},
	}, collected.ConfigFiles)
	assert.Empty(t, collected.EnvVars)
}

func TestPostgresCollectorReturnsEnvVarErrorsWithoutConfigPath(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"postgres"},
		},
		readEnvVarsErr: expectedErr,
	}
	collector := NewPostgres()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Empty(t, reader.readFileCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestPostgresCollectorReturnsReadFileErrors(t *testing.T) {
	expectedErr := errors.New("read failed")
	reader := &postgresCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"},
		},
		readFileErr: expectedErr,
	}
	collector := NewPostgres()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []string{"/etc/postgresql/postgresql.conf"}, reader.readFileCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

type postgresCollectorTestReader struct {
	runtimeCommandline      configfilesdiscoveryimpl.TargetCommandline
	liveProcessCommandlines []configfilesdiscoveryimpl.TargetCommandline
	commandlineErr          error
	runtimeCommandlineCalls int
	processCommandlineCalls int
	readFileCalls           []string
	file                    configfilesdiscoveryimpl.ConfigFile
	readFileErr             error
	env                     map[string]string
	readEnvVarsErr          error
}

func (r *postgresCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *postgresCollectorTestReader) Close() {}

func (r *postgresCollectorTestReader) ReadFile(_ context.Context, path string) (configfilesdiscoveryimpl.ConfigFile, error) {
	r.readFileCalls = append(r.readFileCalls, path)
	if r.readFileErr != nil {
		return configfilesdiscoveryimpl.ConfigFile{}, r.readFileErr
	}
	if r.file.Path == path {
		return r.file, nil
	}
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("file not found")
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
	r.runtimeCommandlineCalls++
	if r.commandlineErr != nil {
		return configfilesdiscoveryimpl.TargetCommandline{}, r.commandlineErr
	}
	return r.runtimeCommandline, nil
}

func (r *postgresCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	r.processCommandlineCalls++
	return r.liveProcessCommandlines
}
