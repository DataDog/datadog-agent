// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisGetConfigPath(t *testing.T) {
	tests := []struct {
		name        string
		commandline configfilesdiscoveryimpl.TargetCommandline
		wantPath    string
		wantOK      bool
	}{
		{
			name: "official docker custom config",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{
					"docker-entrypoint.sh",
					"redis-server",
					"/usr/local/etc/redis/redis.conf",
				},
			},
			wantPath: "/usr/local/etc/redis/redis.conf",
			wantOK:   true,
		},
		{
			name: "explicit redis command",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"redis-server", "/etc/redis/redis.conf"},
			},
			wantPath: "/etc/redis/redis.conf",
			wantOK:   true,
		},
		{
			name: "redis full config",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"redis-server", "/etc/redis/redis-full.conf"},
			},
			wantPath: "/etc/redis/redis-full.conf",
			wantOK:   true,
		},
		{
			name: "relative path",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args:       []string{"redis-server", "redis.conf"},
				WorkingDir: "/usr/local/etc/redis",
			},
			wantPath: "/usr/local/etc/redis/redis.conf",
			wantOK:   true,
		},
		{
			name: "relative path with docker default working dir",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args:       []string{"redis-server", "redis.conf"},
				WorkingDir: "/",
			},
			wantPath: "/redis.conf",
			wantOK:   true,
		},
		{
			name: "arbitrary config filename",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"redis-server", "/usr/local/etc/redis/foo.bar"},
			},
			wantPath: "/usr/local/etc/redis/foo.bar",
			wantOK:   true,
		},
		{
			name: "direct config path without redis server",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"/usr/local/etc/redis/redis.conf"},
			},
		},
		{
			name: "default startup",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"redis-server"},
			},
		},
		{
			name: "flags only",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"redis-server", "--save", "60", "1", "--loglevel", "warning"},
			},
		},
		{
			name: "shell form",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"/bin/sh", "-c", "redis-server /etc/redis/redis.conf"},
			},
			wantPath: "/etc/redis/redis.conf",
			wantOK:   true,
		},
		{
			name: "non redis command",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"nginx", "-c", "/etc/nginx/nginx.conf"},
			},
		},
		{
			name: "relative path without absolute working dir",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"redis-server", "redis.conf"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configArg, gotOK := redisGetConfigArgFromCommandline(tt.commandline.Args)
			var gotPath string
			if gotOK {
				gotPath, gotOK = resolveConfigPath(configArg, tt.commandline.WorkingDir)
			}

			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}

func TestRedisCollectorResolvesAndReadsRelativeProcessConfig(t *testing.T) {
	eventArgs := []string{"redis-server", "redis.conf"}
	eventCommandline := configfilesdiscoveryimpl.TargetCommandline{
		Args:       eventArgs,
		WorkingDir: "/etc/redis",
	}
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{eventCommandline},
		file:                    configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/redis.conf"},
	}

	collector := redisConfigCollector{}
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: eventArgs}))
	assert.True(t, collector.CanCollectFromProcess(eventCommandline))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"redis-server", "/etc/redis/redis.conf"},
	}))

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestIncludeRedisEnvVar(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		want    bool
	}{
		{name: "bitnami config", envName: "REDIS_AOF_ENABLED", want: true},
		{name: "config file", envName: "REDIS_CONF_FILE", want: true},
		{name: "cluster config", envName: "REDIS_CLUSTER_ANNOUNCE_HOSTNAME", want: true},
		{name: "read only image config", envName: "REDIS_DEFAULT_CONF_DIR", want: true},
		{name: "tls certificate path", envName: "REDIS_TLS_CERT_FILE", want: true},
		{name: "fips config", envName: "OPENSSL_FIPS", want: true},
		{name: "unrelated", envName: "UNREQUESTED"},
		{name: "lowercase", envName: "REDIS_port"},
		{name: "password", envName: "REDIS_PASSWORD"},
		{name: "password file", envName: "REDIS_MASTER_PASSWORD_FILE"},
		{name: "password abbreviation", envName: "REDIS_SENTINEL_PWD"},
		{name: "requirepass directive", envName: "REDIS_REQUIREPASS"},
		{name: "masterauth directive", envName: "REDIS_MASTERAUTH"},
		{name: "auth directive", envName: "REDIS_AUTH"},
		{name: "allow empty password", envName: "ALLOW_EMPTY_PASSWORD"},
		{name: "tls key file", envName: "REDIS_TLS_KEY_FILE"},
		{name: "redis cli auth", envName: "REDISCLI_AUTH"},
		{name: "redis args", envName: "REDIS_ARGS"},
		{name: "extra flags", envName: "REDIS_EXTRA_FLAGS"},
		{name: "custom opts", envName: "REDIS_CUSTOM_OPTS"},
		{name: "redis search args", envName: "REDISEARCH_ARGS"},
		{name: "redis json args", envName: "REDISJSON_ARGS"},
		{name: "redis time series args", envName: "REDISTIMESERIES_ARGS"},
		{name: "redis bloom args", envName: "REDISBLOOM_ARGS"},
		{name: "redis url", envName: "REDIS_URL"},
		{name: "sentinel uri", envName: "REDIS_SENTINEL_URI"},
		{name: "cluster dsn", envName: "REDIS_CLUSTER_DSN"},
		{name: "connection string", envName: "REDIS_CONNECTION_STRING"},
		{name: "valkey", envName: "VALKEY_PORT"},
		{name: "dragonfly", envName: "DFLY_PORT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, includeRedisEnvVar(tt.envName))
		})
	}
}

func TestRedisCollectorReadsDetectedConfig(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "/etc/redis/redis.conf"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:      "/etc/redis/redis.conf",
			Content:   []byte("port 6379\n"),
			Truncated: true,
		},
		env: map[string]string{
			"REDIS_PORT_NUMBER": "6379",
			"REDIS_AOF_ENABLED": "yes",
			"UNREQUESTED":       "value",
		},
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configfilesdiscoveryimpl.ConfigFile{
		Path:          "/etc/redis/redis.conf",
		Content:       []byte("port 6379\n"),
		Truncated:     true,
		PayloadFormat: redisConfigPayloadFormat,
	}, collected.ConfigFiles[0])
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "REDIS_AOF_ENABLED", Value: "yes"},
		{Name: "REDIS_PORT_NUMBER", Value: "6379"},
	}, collected.EnvVars)
}

func TestRedisCollectorCollectsEnvOnly(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "--save", "60", "1"},
		},
		env: map[string]string{
			"REDIS_PORT_NUMBER": "6379",
			"REDIS_AOF_ENABLED": "yes",
		},
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "REDIS_AOF_ENABLED", Value: "yes"},
		{Name: "REDIS_PORT_NUMBER", Value: "6379"},
	}, collected.EnvVars)
}

func TestRedisCollectorDoesNotUseMetadataPathsAsConfigFiles(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "--save", "60", "1"},
		},
		env: map[string]string{
			"REDIS_CONF_DIR":       "/opt/bitnami/redis/etc",
			"REDIS_OVERRIDES_FILE": "/opt/bitnami/redis/mounted-etc/overrides.conf",
		},
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "REDIS_CONF_DIR", Value: "/opt/bitnami/redis/etc"},
		{Name: "REDIS_OVERRIDES_FILE", Value: "/opt/bitnami/redis/mounted-etc/overrides.conf"},
	}, collected.EnvVars)
}

func TestRedisCollectorUsesEnvConfigFile(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/opt/bitnami/scripts/redis/run.sh"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/opt/bitnami/redis/etc/redis.conf",
			Content: []byte("port 6379\n"),
		},
		env: map[string]string{
			"REDIS_CONF_FILE":   "/opt/bitnami/redis/etc/redis.conf",
			"REDIS_AOF_ENABLED": "yes",
		},
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/opt/bitnami/redis/etc/redis.conf"}, reader.readFileCalls)
	assert.Equal(t, 1, reader.runtimeCommandlineCalls)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "REDIS_AOF_ENABLED", Value: "yes"},
		{Name: "REDIS_CONF_FILE", Value: "/opt/bitnami/redis/etc/redis.conf"},
	}, collected.EnvVars)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, redisConfigPayloadFormat, collected.ConfigFiles[0].PayloadFormat)
}

func TestRedisCollectorUsesEnvConfigFileWhenCommandlineHasOnlyOptions(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "--save", "60", "1"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path: "/opt/bitnami/redis/etc/redis.conf",
		},
		env: map[string]string{
			"REDIS_CONF_FILE": "/opt/bitnami/redis/etc/redis.conf",
		},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/opt/bitnami/redis/etc/redis.conf"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, redisConfigPayloadFormat, collected.ConfigFiles[0].PayloadFormat)
}

func TestRedisCollectorResolvesRelativeEnvConfigFile(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args:       []string{"/opt/bitnami/scripts/redis/run.sh"},
			WorkingDir: "/opt/bitnami/redis/etc",
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path: "/opt/bitnami/redis/etc/redis.conf",
		},
		env: map[string]string{
			"REDIS_CONF_FILE": "redis.conf",
		},
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/opt/bitnami/redis/etc/redis.conf"}, reader.readFileCalls)
	assert.Equal(t, 1, reader.runtimeCommandlineCalls)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, redisConfigPayloadFormat, collected.ConfigFiles[0].PayloadFormat)
}

func TestRedisCollectorPrefersFirstProcessConfigFileOverEnvConfigFile(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/opt/bitnami/scripts/redis/run.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"redis-server", "/etc/redis/one.conf"}},
			{Args: []string{"redis-server", "/etc/redis/two.conf"}},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path: "/etc/redis/one.conf",
		},
		env: map[string]string{
			"REDIS_CONF_FILE": "/opt/bitnami/redis/etc/redis.conf",
		},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/one.conf"}, reader.readFileCalls)
	assert.Equal(t, 1, reader.runtimeCommandlineCalls)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, "/etc/redis/one.conf", collected.ConfigFiles[0].Path)
	assert.Equal(t, redisConfigPayloadFormat, collected.ConfigFiles[0].PayloadFormat)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "REDIS_CONF_FILE", Value: "/opt/bitnami/redis/etc/redis.conf"},
	}, collected.EnvVars)
}

func TestRedisCollectorPrefersCommandlineConfigFile(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "/etc/redis/redis.conf"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path: "/etc/redis/redis.conf",
		},
		env: map[string]string{
			"REDIS_CONF_FILE": "/opt/bitnami/redis/etc/redis.conf",
		},
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, redisConfigPayloadFormat, collected.ConfigFiles[0].PayloadFormat)
}

func TestRedisCollectorDoesNotCollectSensitiveEnvVars(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "--save", "60", "1"},
		},
		env: map[string]string{
			"REDIS_AOF_ENABLED":          "yes",
			"REDIS_PASSWORD":             "secret",
			"REDIS_MASTER_PASSWORD_FILE": "/run/secrets/master-password",
			"REDIS_SENTINEL_PWD":         "secret",
			"REDIS_REQUIREPASS":          "secret",
			"REDIS_MASTERAUTH":           "master-secret",
			"REDIS_TLS_CERT_FILE":        "/etc/redis/tls.crt",
			"REDIS_TLS_KEY_FILE":         "/run/secrets/redis.key",
			"REDIS_ARGS":                 "--requirepass secret",
			"REDIS_EXTRA_FLAGS":          "--masterauth secret",
			"REDIS_URL":                  "redis://user:secret@redis:6379",
			"ALLOW_EMPTY_PASSWORD":       "yes",
		},
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "REDIS_AOF_ENABLED", Value: "yes"},
		{Name: "REDIS_TLS_CERT_FILE", Value: "/etc/redis/tls.crt"},
	}, collected.EnvVars)
}

func TestRedisCollectorSkipsDefaultsWhenCommandlineHasNoConfigFile(t *testing.T) {
	for _, args := range [][]string{
		{"redis-server"},
		{"redis-server", "--save", "60", "1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			reader := &redisCollectorTestReader{
				runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: args},
				file: configfilesdiscoveryimpl.ConfigFile{
					Path:    "/etc/redis/redis.conf",
					Content: []byte("port 6379\n"),
				},
			}

			collected, err := NewRedis().Collect(context.Background(), reader)

			require.NoError(t, err)
			assert.Empty(t, reader.readFileCalls)
			assert.Empty(t, collected.ConfigFiles)
			assert.Empty(t, collected.EnvVars)
		})
	}
}

func TestRedisCollectorReadsDefaultConfig(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start-redis.sh"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/etc/redis/redis.conf",
			Content: []byte("port 6379\n"),
		},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, redisDefaultConfigPaths, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configfilesdiscoveryimpl.ConfigFile{
		Path:          "/etc/redis/redis.conf",
		Content:       []byte("port 6379\n"),
		PayloadFormat: redisConfigPayloadFormat,
	}, collected.ConfigFiles[0])
}

func TestRedisCollectorSkipsDefaultsWhenLiveProcessHasNoConfigFile(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start-redis.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{{
			Args: []string{"redis-server", "--save", "60", "1"},
		}},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/etc/redis/redis.conf",
			Content: []byte("port 6379\n"),
		},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
}

func TestRedisCollectorUsesFirstConfigAcrossProcesses(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"redis-server", "/etc/redis/redis.conf"}},
			{Args: []string{"redis-server", "/etc/redis/other.conf"}},
			{Args: []string{"redis-server", "relative.conf"}},
		},
		file: configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/redis.conf"},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestRedisCollectorUsesRuntimeConfigBeforeProcessConfig(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "/etc/redis/runtime.conf"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"redis-server", "/etc/redis/process.conf"}},
		},
		file: configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/runtime.conf"},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/runtime.conf"}, reader.readFileCalls)
	assert.Zero(t, reader.processCommandlineCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestRedisCollectorFallsBackToProcessCommandlineOnRuntimeCommandlineError(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &redisCollectorTestReader{
		commandlineErr: expectedErr,
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{{
			Args: []string{"redis-server", "/etc/redis/redis.conf"},
		}},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path: "/etc/redis/redis.conf",
		},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestRedisCollectorReturnsCommandlineErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &redisCollectorTestReader{commandlineErr: expectedErr}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestRedisCollectorReadsDetectedConfigWhenEnvVarsFail(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "/etc/redis/redis.conf"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/etc/redis/redis.conf",
			Content: []byte("port 6379\n"),
		},
		readEnvVarsErr: expectedErr,
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigFile{
		{
			Path:          "/etc/redis/redis.conf",
			Content:       []byte("port 6379\n"),
			PayloadFormat: redisConfigPayloadFormat,
		},
	}, collected.ConfigFiles)
	assert.Empty(t, collected.EnvVars)
}

func TestRedisCollectorReturnsEnvVarErrorsWithoutConfigPath(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "--save", "60", "1"},
		},
		readEnvVarsErr: expectedErr,
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Empty(t, reader.readFileCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestRedisCollectorReturnsReadFileErrors(t *testing.T) {
	expectedErr := errors.New("read failed")
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "/etc/redis/redis.conf"},
		},
		readFileErr: expectedErr,
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestRedisIncludePatterns(t *testing.T) {
	content := []byte(`
# include ignored.conf
InClUdE conf.d/*.conf # trailing comment
include hash#path.conf
include "quoted#path.conf"
include "quoted path.conf"
include 'single\'quote.conf'
include "escaped\ path.conf"
include "hex\x2dpath.conf"
rename-command CONFIG "include not-a-directive.conf"
include-too other.conf
include missing extra
include "unterminated
`)

	patterns, parserOK := redisIncludePatterns(content)

	assert.False(t, parserOK)
	assert.Equal(t, []string{
		"hash#path.conf",
		"quoted#path.conf",
		"quoted path.conf",
		"single'quote.conf",
		"escaped path.conf",
		"hex-path.conf",
	}, patterns)
}

func TestCompileRedisIncludePattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		filePath string
		want     bool
		wantErr  bool
	}{
		{name: "wildcard", pattern: "/etc/redis/conf.d/*.conf", filePath: "/etc/redis/conf.d/redis.conf", want: true},
		{name: "wildcard does not match leading period", pattern: "/etc/redis/conf.d/*.conf", filePath: "/etc/redis/conf.d/.hidden.conf"},
		{name: "question mark does not match leading period", pattern: "/etc/redis/conf.d/?.conf", filePath: "/etc/redis/conf.d/.conf"},
		{name: "bracket does not match leading period", pattern: "/etc/redis/conf.d/[.]hidden.conf", filePath: "/etc/redis/conf.d/.hidden.conf"},
		{name: "explicit period", pattern: "/etc/redis/conf.d/.*.conf", filePath: "/etc/redis/conf.d/.hidden.conf", want: true},
		{name: "escaped explicit period", pattern: `/etc/redis/conf.d/\.*.conf`, filePath: "/etc/redis/conf.d/.hidden.conf", want: true},
		{name: "posix bracket negation matches", pattern: "/etc/redis/conf.d/[!a].conf", filePath: "/etc/redis/conf.d/b.conf", want: true},
		{name: "posix bracket negation rejects", pattern: "/etc/redis/conf.d/[!a].conf", filePath: "/etc/redis/conf.d/a.conf"},
		{name: "malformed bracket", pattern: "/etc/redis/conf.d/[.conf", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := compileRedisIncludePattern(verifyTestConfigFilePattern(t, tt.pattern))
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, matches)
				return
			}
			require.NoError(t, err)

			matched, err := matches(verifyTestConfigFilePath(t, tt.filePath))

			require.NoError(t, err)
			assert.Equal(t, tt.want, matched)
		})
	}
}

func TestRedisCollectorTraversesIncludesDepthFirst(t *testing.T) {
	reader := &redisCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args:       []string{"redis-server", "redis.conf"},
			WorkingDir: "/etc/redis",
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/etc/redis/redis.conf",
			Content: []byte("include conf.d/*.conf\n"),
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/redis/conf.d/a.conf": {Path: "/etc/redis/conf.d/a.conf", Content: []byte("include /etc/redis/nested.conf\n")},
			"/etc/redis/conf.d/b.conf": {Path: "/etc/redis/conf.d/b.conf", Content: []byte("include /etc/redis/conf.d/a.conf\n")},
			"/etc/redis/nested.conf":   {Path: "/etc/redis/nested.conf", Content: []byte("port 6380\n")},
		},
		findFiles: map[string][]string{
			"/etc/redis/conf.d/*.conf": {"/etc/redis/conf.d/a.conf", "/etc/redis/conf.d/b.conf"},
			"/etc/redis/nested.conf":   {"/etc/redis/nested.conf"},
			"/etc/redis/conf.d/a.conf": {"/etc/redis/conf.d/a.conf"},
		},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	require.Len(t, collected.ConfigFiles, 4)
	assert.Equal(t, []string{
		"/etc/redis/redis.conf",
		"/etc/redis/conf.d/a.conf",
		"/etc/redis/nested.conf",
		"/etc/redis/conf.d/b.conf",
	}, redisConfigFilePaths(collected.ConfigFiles))
	for _, file := range collected.ConfigFiles {
		assert.Equal(t, redisConfigPayloadFormat, file.PayloadFormat)
	}
}

func TestRedisCollectorSkipsRelativeIncludesWithoutReliableWorkingDir(t *testing.T) {
	root := configfilesdiscoveryimpl.ConfigFile{
		Path:    "/etc/redis/redis.conf",
		Content: []byte("include relative.conf\ninclude /etc/redis/absolute.conf\n"),
	}
	reader := &redisCollectorTestReader{
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/redis/absolute.conf": {Path: "/etc/redis/absolute.conf"},
		},
		findFiles: map[string][]string{
			"/etc/redis/absolute.conf": {"/etc/redis/absolute.conf"},
		},
	}

	files, limited := collectRedisConfigFiles(context.Background(), reader, root, verifyTestConfigFilePath(t, root.Path), "")

	assert.False(t, limited)
	assert.Equal(t, []string{"/etc/redis/redis.conf", "/etc/redis/absolute.conf"}, redisConfigFilePaths(files))
}

func TestResolveRedisIncludePatternRejectsUnsafePaths(t *testing.T) {
	tests := []string{
		"../outside.conf",
		"/etc/redis/../outside.conf",
		"/etc/other/outside.conf",
		"/etc/redis/**/recursive.conf",
		"/etc/redis/control\n.conf",
		"/etc/redis/nul\x00.conf",
	}
	for _, include := range tests {
		t.Run(fmt.Sprintf("%q", include), func(t *testing.T) {
			pattern, err := resolveRedisIncludePattern(include, "/etc/redis", "/etc/redis")
			require.Error(t, err)
			assert.Empty(t, pattern)
		})
	}
}

func TestRedisCollectorSkipsUnreadableUnmatchedAndUnsafeIncludes(t *testing.T) {
	root := configfilesdiscoveryimpl.ConfigFile{
		Path: "/etc/redis/redis.conf",
		Content: []byte(strings.Join([]string{
			"include /etc/redis/unreadable.conf",
			"include /etc/redis/unreadable.conf",
			"include /etc/redis/unmatched*.conf",
			"include /outside.conf",
		}, "\n")),
	}
	reader := &redisCollectorTestReader{
		findFiles: map[string][]string{
			"/etc/redis/unreadable.conf": {"/etc/redis/unreadable.conf"},
		},
	}

	files, limited := collectRedisConfigFiles(context.Background(), reader, root, verifyTestConfigFilePath(t, root.Path), "/etc/redis")

	assert.False(t, limited)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, redisConfigFilePaths(files))
	assert.Equal(t, []string{"/etc/redis/unreadable.conf"}, reader.readFileCalls)
}

func TestRedisCollectorEnforcesIncludeDepthLimit(t *testing.T) {
	root := configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/0.conf", Content: []byte("include /etc/redis/1.conf\n")}
	reader := &redisCollectorTestReader{
		files:     make(map[string]configfilesdiscoveryimpl.ConfigFile),
		findFiles: make(map[string][]string),
	}
	for i := 1; i <= redisMaxIncludeDepth; i++ {
		filePath := fmt.Sprintf("/etc/redis/%d.conf", i)
		nextPath := fmt.Sprintf("/etc/redis/%d.conf", i+1)
		reader.files[filePath] = configfilesdiscoveryimpl.ConfigFile{Path: filePath, Content: []byte("include " + nextPath + "\n")}
		reader.findFiles[filePath] = []string{filePath}
	}

	files, limited := collectRedisConfigFiles(context.Background(), reader, root, verifyTestConfigFilePath(t, root.Path), "/etc/redis")

	assert.True(t, limited)
	assert.Len(t, files, redisMaxIncludeDepth)
	assert.Equal(t, "/etc/redis/9.conf", files[len(files)-1].Path)
}

func TestRedisCollectorEnforcesFileLimit(t *testing.T) {
	root := configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/redis.conf", Content: []byte("include /etc/redis/conf.d/*.conf\n")}
	reader := &redisCollectorTestReader{
		files:     make(map[string]configfilesdiscoveryimpl.ConfigFile),
		findFiles: map[string][]string{"/etc/redis/conf.d/*.conf": {}},
	}
	for i := 0; i < redisMaxConfigFiles+5; i++ {
		filePath := fmt.Sprintf("/etc/redis/conf.d/%02d.conf", i)
		reader.files[filePath] = configfilesdiscoveryimpl.ConfigFile{Path: filePath}
		reader.findFiles["/etc/redis/conf.d/*.conf"] = append(reader.findFiles["/etc/redis/conf.d/*.conf"], filePath)
	}

	files, limited := collectRedisConfigFiles(context.Background(), reader, root, verifyTestConfigFilePath(t, root.Path), "/etc/redis")

	assert.True(t, limited)
	assert.Len(t, files, redisMaxConfigFiles)
}

func TestRedisCollectorLimitsIncludeSearches(t *testing.T) {
	var includes strings.Builder
	for i := 0; i <= redisMaxIncludeSearches; i++ {
		fmt.Fprintf(&includes, "include /etc/redis/missing-%03d.conf\n", i)
	}
	root := configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/redis.conf", Content: []byte(includes.String())}
	reader := &redisCollectorTestReader{}

	files, limited := collectRedisConfigFiles(context.Background(), reader, root, verifyTestConfigFilePath(t, root.Path), "/etc/redis")

	assert.True(t, limited)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, redisConfigFilePaths(files))
	assert.Len(t, reader.findFilesCalls, redisMaxIncludeSearches)
}

func TestRedisCollectorDeduplicatesBeforeLimitingMatches(t *testing.T) {
	const pattern = "/etc/redis/*.conf"
	root := configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/00-root.conf", Content: []byte("include " + pattern + "\n")}
	reader := &redisCollectorTestReader{
		files:     make(map[string]configfilesdiscoveryimpl.ConfigFile),
		findFiles: map[string][]string{pattern: {root.Path}},
	}
	for i := 1; i < redisMaxConfigFiles; i++ {
		filePath := fmt.Sprintf("/etc/redis/%02d.conf", i)
		reader.files[filePath] = configfilesdiscoveryimpl.ConfigFile{Path: filePath}
		reader.findFiles[pattern] = append(reader.findFiles[pattern], filePath)
	}

	files, limited := collectRedisConfigFiles(context.Background(), reader, root, verifyTestConfigFilePath(t, root.Path), "/etc/redis")

	assert.False(t, limited)
	assert.Len(t, files, redisMaxConfigFiles)
	assert.Equal(t, "/etc/redis/31.conf", files[len(files)-1].Path)
}

func TestRedisCollectorTruncatesAtAggregateLimit(t *testing.T) {
	rootContent := append([]byte("include /etc/redis/conf.d/*.conf\n"), bytes.Repeat([]byte("r"), 100)...)
	root := configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/redis.conf", Content: rootContent}
	reader := &redisCollectorTestReader{
		files:     make(map[string]configfilesdiscoveryimpl.ConfigFile),
		findFiles: map[string][]string{"/etc/redis/conf.d/*.conf": {}},
	}
	for i := 0; i < 4; i++ {
		filePath := fmt.Sprintf("/etc/redis/conf.d/%d.conf", i)
		reader.files[filePath] = configfilesdiscoveryimpl.ConfigFile{Path: filePath, Content: bytes.Repeat([]byte{byte('a' + i)}, 1024*1024)}
		reader.findFiles["/etc/redis/conf.d/*.conf"] = append(reader.findFiles["/etc/redis/conf.d/*.conf"], filePath)
	}

	files, limited := collectRedisConfigFiles(context.Background(), reader, root, verifyTestConfigFilePath(t, root.Path), "/etc/redis")

	assert.True(t, limited)
	require.Len(t, files, 5)
	last := files[len(files)-1]
	assert.True(t, last.Truncated)
	assert.Len(t, last.Content, redisMaxAggregateBytes-len(rootContent)-3*1024*1024)
	total := 0
	for _, file := range files {
		total += len(file.Content)
	}
	assert.Equal(t, redisMaxAggregateBytes, total)
}

// redisConfigFilePaths returns the paths of files in their existing order.
func redisConfigFilePaths(files []configfilesdiscoveryimpl.ConfigFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

type redisCollectorTestReader struct {
	runtimeCommandline      configfilesdiscoveryimpl.TargetCommandline
	liveProcessCommandlines []configfilesdiscoveryimpl.TargetCommandline
	commandlineErr          error
	runtimeCommandlineCalls int
	processCommandlineCalls int
	readFileCalls           []string
	file                    configfilesdiscoveryimpl.ConfigFile
	readFileErr             error
	files                   map[string]configfilesdiscoveryimpl.ConfigFile
	findFiles               map[string][]string
	findFilesCalls          []string
	findFilesLimited        map[string]bool
	findFilesErr            map[string]error
	env                     map[string]string
	readEnvVarsErr          error
}

func (r *redisCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *redisCollectorTestReader) Close() {}

func (r *redisCollectorTestReader) ReadFile(_ context.Context, path configfilesdiscoveryimpl.VerifiedConfigFilePath) (configfilesdiscoveryimpl.ConfigFile, error) {
	r.readFileCalls = append(r.readFileCalls, path.String())
	if r.readFileErr != nil {
		return configfilesdiscoveryimpl.ConfigFile{}, r.readFileErr
	}
	if r.file.Path == path.String() {
		return r.file, nil
	}
	if file, found := r.files[path.String()]; found {
		return file, nil
	}
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("file not found")
}

// FindFiles returns configured paths accepted by matches, bounded by
// maxMatches, and reports whether additional paths were omitted.
func (r *redisCollectorTestReader) FindFiles(
	_ context.Context,
	pattern configfilesdiscoveryimpl.VerifiedConfigFilePattern,
	maxMatches int,
	matches configfilesdiscoveryimpl.ConfigFilePathMatcher,
) ([]configfilesdiscoveryimpl.VerifiedConfigFilePath, bool, error) {
	patternValue := pattern.String()
	r.findFilesCalls = append(r.findFilesCalls, patternValue)
	if err := r.findFilesErr[patternValue]; err != nil {
		return nil, false, err
	}
	var paths []configfilesdiscoveryimpl.VerifiedConfigFilePath
	for _, filePath := range r.findFiles[patternValue] {
		verifiedPath, err := configfilesdiscoveryimpl.VerifyConfigFilePath(configfilesdiscoveryimpl.UnverifiedConfigFilePath(filePath))
		if err != nil {
			return nil, false, err
		}
		matched, err := matches(verifiedPath)
		if err != nil {
			return nil, false, err
		}
		if matched {
			paths = append(paths, verifiedPath)
		}
	}
	limited := r.findFilesLimited[patternValue]
	if len(paths) > maxMatches {
		paths = paths[:maxMatches]
		limited = true
	}
	return paths, limited, nil
}

func (r *redisCollectorTestReader) ReadEnvVars(_ context.Context, predicate configfilesdiscoveryimpl.ConfigEnvVarPredicate) (map[string]string, error) {
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

func (r *redisCollectorTestReader) ReadRuntimeCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	r.runtimeCommandlineCalls++
	if r.commandlineErr != nil {
		return configfilesdiscoveryimpl.TargetCommandline{}, r.commandlineErr
	}
	return r.runtimeCommandline, nil
}

func (r *redisCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	r.processCommandlineCalls++
	return r.liveProcessCommandlines
}
