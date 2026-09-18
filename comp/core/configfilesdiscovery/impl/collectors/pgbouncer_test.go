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
		{"PGBOUNCER_CONF_DIR", true},
		{"PGBOUNCER_CONF_FILE", true},
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

func TestPgbouncerCollectorCanCollectFromProcess(t *testing.T) {
	collector := NewPgbouncer()

	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"pgbouncer", "/etc/pgbouncer/pgbouncer.ini"}}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"/usr/bin/pgbouncer", "/etc/pgbouncer/pgbouncer.ini"}}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"/bin/sh", "-c", "pgbouncer /etc/pgbouncer/pgbouncer.ini"}}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"pgbouncer-exporter"}}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{}))
}

func TestPgbouncerCollectorReturnsEnvReadError(t *testing.T) {
	expectedErr := errors.New("environment unavailable")
	_, err := NewPgbouncer().Collect(context.Background(), &pgbouncerCollectorTestReader{readEnvVarsErr: expectedErr})
	require.ErrorIs(t, err, expectedErr)
}

func TestPgbouncerGetConfigArgFromCommandline(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPath string
		wantOK   bool
	}{
		{name: "explicit config file", args: []string{"pgbouncer", "/etc/pgbouncer/pgbouncer.ini"}, wantPath: "/etc/pgbouncer/pgbouncer.ini", wantOK: true},
		{name: "resolved binary path", args: []string{"/usr/bin/pgbouncer", "/etc/pgbouncer/pgbouncer.ini"}, wantPath: "/etc/pgbouncer/pgbouncer.ini", wantOK: true},
		{name: "daemon flag before config", args: []string{"pgbouncer", "-d", "/etc/pgbouncer/pgbouncer.ini"}, wantPath: "/etc/pgbouncer/pgbouncer.ini", wantOK: true},
		{name: "long daemon flag before config", args: []string{"pgbouncer", "--daemon", "/etc/pgbouncer/pgbouncer.ini"}, wantPath: "/etc/pgbouncer/pgbouncer.ini", wantOK: true},
		{name: "user flag with separate value", args: []string{"pgbouncer", "-u", "postgres", "/etc/pgbouncer/pgbouncer.ini"}, wantPath: "/etc/pgbouncer/pgbouncer.ini", wantOK: true},
		{name: "long user flag with inline value", args: []string{"pgbouncer", "--user=postgres", "/etc/pgbouncer/pgbouncer.ini"}, wantPath: "/etc/pgbouncer/pgbouncer.ini", wantOK: true},
		{name: "shell wrapper", args: []string{"/bin/sh", "-c", "pgbouncer /etc/pgbouncer/pgbouncer.ini"}, wantPath: "/etc/pgbouncer/pgbouncer.ini", wantOK: true},
		{name: "bitnami config path", args: []string{"pgbouncer", "/opt/bitnami/pgbouncer/conf/pgbouncer.ini"}, wantPath: "/opt/bitnami/pgbouncer/conf/pgbouncer.ini", wantOK: true},
		{name: "no config file", args: []string{"pgbouncer"}},
		{name: "version flag only", args: []string{"pgbouncer", "--version"}},
		{name: "unrecognized flag is not guessed at", args: []string{"pgbouncer", "--unknown-flag", "/etc/pgbouncer/pgbouncer.ini"}},
		{name: "non pgbouncer command", args: []string{"redis-server", "/etc/redis/redis.conf"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, ok := pgbouncerGetConfigArgFromCommandline(tt.args)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantPath, path)
		})
	}
}

func TestPgbouncerFallbackConfigArg(t *testing.T) {
	assert.Equal(t, "/opt/bitnami/pgbouncer/conf/pgbouncer.ini", pgbouncerFallbackConfigArg([]configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "PGBOUNCER_CONF_DIR", Value: "/opt/bitnami/pgbouncer/conf"},
	}))
	assert.Equal(t, "/custom/pgbouncer.ini", pgbouncerFallbackConfigArg([]configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "PGBOUNCER_CONF_DIR", Value: "/opt/bitnami/pgbouncer/conf"},
		{Name: "PGBOUNCER_CONF_FILE", Value: "/custom/pgbouncer.ini"},
	}))
	assert.Equal(t, "", pgbouncerFallbackConfigArg(nil))
}

func TestPgbouncerCollectorReadsConfigFromExplicitCommandline(t *testing.T) {
	const configPath = "/etc/pgbouncer/pgbouncer.ini"
	reader := &pgbouncerCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"pgbouncer", configPath},
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			configPath: {Path: configPath, Content: []byte("[databases]\nverifydb = host=postgres port=5432 auth_user=verifydb\n[pgbouncer]\npool_mode = transaction\n")},
		},
		env: map[string]string{"POOL_MODE": "transaction"},
	}

	collected, err := NewPgbouncer().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{configPath}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configfilesdiscoveryimpl.ConfigFile{
		Path:          configPath,
		Content:       []byte("[databases]\nverifydb = host=postgres port=5432 auth_user=verifydb\n[pgbouncer]\npool_mode = transaction\n"),
		PayloadFormat: pgbouncerConfigPayloadFormat,
	}, collected.ConfigFiles[0])
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{{Name: "POOL_MODE", Value: "transaction"}}, collected.EnvVars)
}

func TestPgbouncerCollectorSkipsDefaultsWhenCommandlineHasNoConfigFile(t *testing.T) {
	reader := &pgbouncerCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{"pgbouncer"}},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/pgbouncer/pgbouncer.ini": {Path: "/etc/pgbouncer/pgbouncer.ini", Content: []byte("pool_mode = transaction\n")},
		},
	}

	collected, err := NewPgbouncer().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Empty(t, collected.EnvVars)
}

func TestPgbouncerCollectorReadsDefaultConfigWhenCommandlineIsOpaque(t *testing.T) {
	reader := &pgbouncerCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/entrypoint.sh"},
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/pgbouncer/pgbouncer.ini": {Path: "/etc/pgbouncer/pgbouncer.ini", Content: []byte("pool_mode = transaction\n")},
		},
	}

	collected, err := NewPgbouncer().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, pgbouncerDefaultConfigPaths, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configfilesdiscoveryimpl.ConfigFile{
		Path:          "/etc/pgbouncer/pgbouncer.ini",
		Content:       []byte("pool_mode = transaction\n"),
		PayloadFormat: pgbouncerConfigPayloadFormat,
	}, collected.ConfigFiles[0])
}

func TestPgbouncerCollectorSkipsDefaultsWhenBothPathsExist(t *testing.T) {
	reader := &pgbouncerCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/entrypoint.sh"},
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/pgbouncer/pgbouncer.ini":              {Path: "/etc/pgbouncer/pgbouncer.ini"},
			"/opt/bitnami/pgbouncer/conf/pgbouncer.ini": {Path: "/opt/bitnami/pgbouncer/conf/pgbouncer.ini"},
		},
	}

	collected, err := NewPgbouncer().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, collected.ConfigFiles)
}

func TestPgbouncerCollectorUsesBitnamiConfDirFallback(t *testing.T) {
	const configPath = "/opt/bitnami/pgbouncer/conf/pgbouncer.ini"
	reader := &pgbouncerCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/opt/bitnami/scripts/pgbouncer/run.sh"},
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			configPath: {Path: configPath, Content: []byte("pool_mode = transaction\n")},
		},
		env: map[string]string{"PGBOUNCER_CONF_DIR": "/opt/bitnami/pgbouncer/conf"},
	}

	collected, err := NewPgbouncer().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{configPath}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configPath, collected.ConfigFiles[0].Path)
}

func TestPgbouncerCollectorUsesBitnamiConfFileFallback(t *testing.T) {
	const configPath = "/custom/pgbouncer.ini"
	reader := &pgbouncerCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/opt/bitnami/scripts/pgbouncer/run.sh"},
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			configPath: {Path: configPath, Content: []byte("pool_mode = transaction\n")},
		},
		env: map[string]string{
			"PGBOUNCER_CONF_DIR":  "/opt/bitnami/pgbouncer/conf",
			"PGBOUNCER_CONF_FILE": configPath,
		},
	}

	collected, err := NewPgbouncer().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{configPath}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configPath, collected.ConfigFiles[0].Path)
}

func TestPgbouncerCollectorNeverReadsUserlistOrAuthFiles(t *testing.T) {
	for _, p := range pgbouncerDefaultConfigPaths {
		assert.NotContains(t, p, "userlist")
	}
}

func TestPgbouncerCollectorReturnsReadFileErrors(t *testing.T) {
	expectedErr := errors.New("read failed")
	reader := &pgbouncerCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"pgbouncer", "/etc/pgbouncer/pgbouncer.ini"},
		},
		readFileErr: expectedErr,
	}

	collected, err := NewPgbouncer().Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

type pgbouncerCollectorTestReader struct {
	env                     map[string]string
	readEnvVarsErr          error
	commandline             configfilesdiscoveryimpl.TargetCommandline
	commandlineErr          error
	liveProcessCommandlines []configfilesdiscoveryimpl.TargetCommandline
	files                   map[string]configfilesdiscoveryimpl.ConfigFile
	readFileCalls           []string
	readFileErr             error
}

func (r *pgbouncerCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *pgbouncerCollectorTestReader) Close() {}

func (r *pgbouncerCollectorTestReader) ReadFile(_ context.Context, path string) (configfilesdiscoveryimpl.ConfigFile, error) {
	r.readFileCalls = append(r.readFileCalls, path)
	if r.readFileErr != nil {
		return configfilesdiscoveryimpl.ConfigFile{}, r.readFileErr
	}
	if file, found := r.files[path]; found {
		return file, nil
	}
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("not found")
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
	if r.commandlineErr != nil {
		return configfilesdiscoveryimpl.TargetCommandline{}, r.commandlineErr
	}
	return r.commandline, nil
}

func (r *pgbouncerCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	return r.liveProcessCommandlines
}
