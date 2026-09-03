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
		{name: "server port", envName: "PGPORT", want: true},
		{name: "client encoding", envName: "PGCLIENTENCODING", want: true},
		{name: "date style", envName: "PGDATESTYLE", want: true},
		{name: "oom adjustment value", envName: "PG_OOM_ADJUST_VALUE", want: true},
		{name: "database name", envName: "POSTGRES_DB", want: true},
		{name: "user", envName: "POSTGRES_USER", want: true},
		{name: "host auth method is not allow-listed", envName: "POSTGRES_HOST_AUTH_METHOD"},
		{name: "initdb waldir", envName: "POSTGRES_INITDB_WALDIR", want: true},
		{name: "bitnami postgres alias", envName: "POSTGRES_PORT_NUMBER", want: true},
		{name: "bitnami data directory", envName: "POSTGRESQL_DATA_DIR", want: true},
		{name: "bitnami database name", envName: "POSTGRESQL_DATABASE", want: true},
		{name: "bitnami user", envName: "POSTGRESQL_USERNAME", want: true},
		{name: "bitnami initdb waldir", envName: "POSTGRESQL_INITDB_WAL_DIR", want: true},
		{name: "bitnami maximum connections", envName: "POSTGRESQL_MAX_CONNECTIONS", want: true},
		{name: "bitnami password encryption algorithm", envName: "POSTGRESQL_PASSWORD_ENCRYPTION", want: true},
		{name: "bitnami tls enabled", envName: "POSTGRESQL_ENABLE_TLS", want: true},
		{name: "bitnami ldap server", envName: "POSTGRESQL_LDAP_SERVER", want: true},
		{name: "bitnami configuration file", envName: "POSTGRESQL_CONF_FILE", want: true},
		{name: "bitnami configuration namespace", envName: "POSTGRESQL_FUTURE_SAFE_SETTING", want: true},
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
		{name: "bitnami password", envName: "POSTGRESQL_PASSWORD"},
		{name: "bitnami password file", envName: "POSTGRESQL_PASSWORD_FILE"},
		{name: "bitnami extra flags", envName: "POSTGRESQL_EXTRA_FLAGS"},
		{name: "bitnami ldap url", envName: "POSTGRESQL_LDAP_URL"},
		{name: "bitnami custom url", envName: "POSTGRESQL_CUSTOM_URL"},
		{name: "bitnami connection string", envName: "POSTGRESQL_CONNECTION_STRING"},
		{name: "bitnami ldap passfile path", envName: "POSTGRESQL_LDAP_BIND_PASSFILE_PATH", want: true},
		{name: "bitnami replication passfile path", envName: "POSTGRESQL_REPLICATION_PASSFILE_PATH", want: true},
		{name: "bitnami ldap use passfile", envName: "POSTGRESQL_LDAP_BIND_USE_PASSFILE", want: true},
		{name: "bitnami replication use passfile", envName: "POSTGRESQL_REPLICATION_USE_PASSFILE", want: true},
		{name: "bitnami hba auth method", envName: "POSTGRESQL_PGHBA_AUTH_METHOD"},
		{name: "bitnami tls key file", envName: "POSTGRESQL_TLS_KEY_FILE"},
		{name: "bitnami tls ca file", envName: "POSTGRESQL_TLS_CA_FILE", want: true},
		{name: "bitnami tls crl file", envName: "POSTGRESQL_TLS_CRL_FILE", want: true},
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
			"POSTGRES_USER":                  "app",
			"PGDATA":                         "/var/lib/postgresql/data",
			"PGPORT":                         "5432",
			"PG_OOM_ADJUST_VALUE":            "0",
			"POSTGRES_DB":                    "app",
			"POSTGRES_PORT_NUMBER":           "5432",
			"POSTGRESQL_DATA_DIR":            "/bitnami/postgresql/data",
			"POSTGRESQL_DATABASE":            "bitnami-app",
			"POSTGRESQL_USERNAME":            "bitnami-app",
			"POSTGRESQL_INITDB_WAL_DIR":      "/bitnami/postgresql/wal",
			"POSTGRESQL_MAX_CONNECTIONS":     "200",
			"POSTGRESQL_ENABLE_TLS":          "yes",
			"POSTGRESQL_LDAP_SERVER":         "ldap.example.test",
			"POSTGRESQL_CONF_FILE":           "/opt/bitnami/postgresql/conf/postgresql.conf",
			"POSTGRESQL_LDAP_URL":            "ldap://user:password@ldap.example.test",
			"POSTGRESQL_PASSWORD_ENCRYPTION": "scram-sha-256",
			"POSTGRESQL_PASSWORD":            "must-not-be-forwarded",
			"UNREQUESTED":                    "value",
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/opt/bitnami/postgresql/conf/postgresql.conf": {
				Path:    "/opt/bitnami/postgresql/conf/postgresql.conf",
				Content: []byte("max_connections = 200\nshared_preload_libraries = 'pg_stat_statements'\n"),
			},
		},
	}
	collector := NewPostgres()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "PGDATA", Value: "/var/lib/postgresql/data"},
		{Name: "PGPORT", Value: "5432"},
		{Name: "PG_OOM_ADJUST_VALUE", Value: "0"},
		{Name: "POSTGRESQL_CONF_FILE", Value: "/opt/bitnami/postgresql/conf/postgresql.conf"},
		{Name: "POSTGRESQL_DATABASE", Value: "bitnami-app"},
		{Name: "POSTGRESQL_DATA_DIR", Value: "/bitnami/postgresql/data"},
		{Name: "POSTGRESQL_ENABLE_TLS", Value: "yes"},
		{Name: "POSTGRESQL_INITDB_WAL_DIR", Value: "/bitnami/postgresql/wal"},
		{Name: "POSTGRESQL_LDAP_SERVER", Value: "ldap.example.test"},
		{Name: "POSTGRESQL_MAX_CONNECTIONS", Value: "200"},
		{Name: "POSTGRESQL_PASSWORD_ENCRYPTION", Value: "scram-sha-256"},
		{Name: "POSTGRESQL_USERNAME", Value: "bitnami-app"},
		{Name: "POSTGRES_DB", Value: "app"},
		{Name: "POSTGRES_PORT_NUMBER", Value: "5432"},
		{Name: "POSTGRES_USER", Value: "app"},
	}, collected.EnvVars)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configfilesdiscoveryimpl.ConfigFile{
		Path:          "/opt/bitnami/postgresql/conf/postgresql.conf",
		Content:       []byte("max_connections = 200\nshared_preload_libraries = 'pg_stat_statements'\n"),
		PayloadFormat: postgresConfigPayloadFormat,
	}, collected.ConfigFiles[0])
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

func TestPostgresCollectorReadsDetectedConfigWhenEnvVarsFail(t *testing.T) {
	const configPath = "/etc/postgresql/postgresql.conf"
	reader := &postgresCollectorTestReader{
		readEnvVarsErr: errors.New("env unavailable"),
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"postgres", "-c", "config_file=" + configPath},
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			configPath: {Path: configPath, Content: []byte("max_connections = 200\n")},
		},
	}

	collected, err := NewPostgres().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{configPath}, reader.readFileCalls)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigFile{{
		Path:          configPath,
		Content:       []byte("max_connections = 200\n"),
		PayloadFormat: postgresConfigPayloadFormat,
	}}, collected.ConfigFiles)
	assert.Empty(t, collected.EnvVars)
}

func TestPostgresCollectorReadsExplicitConfigAfterDockerEntrypoint(t *testing.T) {
	const explicitConfigPath = "/etc/postgresql/custom.conf"
	const fallbackConfigPath = "/var/lib/postgresql/data/postgresql.conf"
	reader := &postgresCollectorTestReader{
		env: map[string]string{
			"PGDATA": "/var/lib/postgresql/data",
		},
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/docker-entrypoint.sh", "postgres", "-c", "config_file=" + explicitConfigPath},
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			explicitConfigPath: {Path: explicitConfigPath, Content: []byte("max_connections = 200\n")},
			fallbackConfigPath: {Path: fallbackConfigPath, Content: []byte("max_connections = 100\n")},
		},
	}

	collected, err := NewPostgres().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{explicitConfigPath}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, explicitConfigPath, collected.ConfigFiles[0].Path)
}

func TestPostgresCollectorSkipsClientCommandline(t *testing.T) {
	const backupConfigPath = "/backup/postgresql.conf"
	reader := &postgresCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"pg_basebackup", "-U", "postgres", "-D", "/backup"},
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			backupConfigPath: {Path: backupConfigPath, Content: []byte("max_connections = 200\n")},
		},
	}

	collected, err := NewPostgres().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, collected)
	assert.Empty(t, reader.readFileCalls)
}

func TestPostgresCollectorCanCollectFromProcess(t *testing.T) {
	collector := postgresConfigCollector{}
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"postgres"},
	}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"},
	}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"docker-entrypoint.sh", "postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"pg_basebackup", "-U", "postgres", "-D", "/backup"},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{}))
}

func TestPostgresGetConfigArgFromCommandline(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPath string
		wantOK   bool
	}{
		{name: "explicit config file", args: []string{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"}, wantPath: "/etc/postgresql/postgresql.conf", wantOK: true},
		{name: "long explicit config file", args: []string{"postgres", "--config-file=/etc/postgresql/postgresql.conf"}, wantPath: "/etc/postgresql/postgresql.conf", wantOK: true},
		{name: "last explicit config file wins", args: []string{"postgres", "-c", "config_file=/one/postgresql.conf", "-c", "config_file=/two/postgresql.conf"}, wantPath: "/two/postgresql.conf", wantOK: true},
		{name: "data directory", args: []string{"postgres", "-D", "/var/lib/postgresql/data"}, wantPath: "/var/lib/postgresql/data/postgresql.conf", wantOK: true},
		{name: "attached data directory", args: []string{"postgres", "-D/var/lib/postgresql/data"}, wantPath: "/var/lib/postgresql/data/postgresql.conf", wantOK: true},
		{name: "last data directory wins", args: []string{"postgres", "-D", "/one", "-D", "/two"}, wantPath: "/two/postgresql.conf", wantOK: true},
		{name: "explicit config file wins over data directory", args: []string{"postgres", "-D", "/var/lib/postgresql/data", "-c", "config_file=/etc/postgresql/postgresql.conf"}, wantPath: "/etc/postgresql/postgresql.conf", wantOK: true},
		{name: "shell wrapper", args: []string{"/bin/sh", "-c", "postgres -D /var/lib/postgresql/data"}, wantPath: "/var/lib/postgresql/data/postgresql.conf", wantOK: true},
		{name: "official docker entrypoint", args: []string{"docker-entrypoint.sh", "postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"}, wantPath: "/etc/postgresql/postgresql.conf", wantOK: true},
		{name: "client command with postgres user", args: []string{"pg_basebackup", "-U", "postgres", "-D", "/backup"}},
		{name: "no path", args: []string{"postgres"}},
		{name: "non postgres command", args: []string{"redis-server", "/etc/redis/redis.conf"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, ok := postgresGetConfigArgFromCommandline(tt.args)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantPath, path)
		})
	}
}

func TestPostgresFallbackConfigArg(t *testing.T) {
	assert.Equal(t, "/opt/bitnami/postgresql/conf/postgresql.conf", postgresFallbackConfigArg([]configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "PGDATA", Value: "/var/lib/postgresql/data"},
		{Name: "POSTGRESQL_CONF_FILE", Value: "/opt/bitnami/postgresql/conf/postgresql.conf"},
	}))
	assert.Equal(t, "/var/lib/postgresql/data/postgresql.conf", postgresFallbackConfigArg([]configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "PGDATA", Value: "/var/lib/postgresql/data"},
	}))
}

type postgresCollectorTestReader struct {
	env            map[string]string
	readEnvVarsErr error
	commandline    configfilesdiscoveryimpl.TargetCommandline
	files          map[string]configfilesdiscoveryimpl.ConfigFile
	readFileCalls  []string
}

func (r *postgresCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *postgresCollectorTestReader) Close() {}

func (r *postgresCollectorTestReader) ReadFile(_ context.Context, path string) (configfilesdiscoveryimpl.ConfigFile, error) {
	r.readFileCalls = append(r.readFileCalls, path)
	if file, found := r.files[path]; found {
		return file, nil
	}
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("not found")
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
	return r.commandline, nil
}

func (r *postgresCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	return nil
}
