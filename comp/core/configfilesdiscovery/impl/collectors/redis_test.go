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

func TestRedisCollectorMatchesAndReadsRelativeProcessConfig(t *testing.T) {
	eventArgs := []string{"redis-server", "redis.conf"}
	reader := &redisCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{{
			Args:       eventArgs,
			WorkingDir: "/etc/redis",
		}},
		file: configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/redis.conf"},
	}

	configArg, matched := redisGetConfigArgFromCommandline(eventArgs)
	require.True(t, matched)
	_, resolved := resolveConfigPath(configArg, "")
	assert.False(t, resolved)
	require.True(t, redisConfigCollector{}.MatchesCommandline(eventArgs))

	collected, err := redisConfigCollector{}.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestRedisCollectorReadsDetectedConfig(t *testing.T) {
	reader := &redisCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "/etc/redis/redis.conf"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:      "/etc/redis/redis.conf",
			Content:   []byte("port 6379\n"),
			Truncated: true,
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
	assert.Empty(t, collected.EnvVars)
}

func TestRedisCollectorSkipsWhenNoConfigPathIsDetected(t *testing.T) {
	reader := &redisCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "--save", "60", "1"},
		},
	}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Empty(t, collected.EnvVars)
}

func TestRedisCollectorReadsUniqueConfigAcrossProcesses(t *testing.T) {
	reader := &redisCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"redis-server", "/etc/redis/redis.conf"}},
			{Args: []string{"redis-server", "/etc/redis/redis.conf"}},
		},
		file: configfilesdiscoveryimpl.ConfigFile{Path: "/etc/redis/redis.conf"},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestRedisCollectorSkipsConflictingProcessConfigPaths(t *testing.T) {
	reader := &redisCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"redis-server", "/etc/redis/redis.conf"}},
			{Args: []string{"redis-server", "/etc/redis/other.conf"}},
		},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
}

func TestRedisCollectorSkipsUnresolvedMatchingProcessConfigPath(t *testing.T) {
	reader := &redisCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"redis-server", "/etc/redis/redis.conf"}},
			{Args: []string{"redis-server", "redis.conf"}},
		},
	}

	collected, err := NewRedis().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
}

func TestRedisCollectorUsesRuntimeConfigBeforeProcessConfig(t *testing.T) {
	reader := &redisCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"redis-server", "/etc/redis/runtime.conf"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{
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

func TestRedisCollectorReturnsCommandlineErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &redisCollectorTestReader{commandlineErr: expectedErr}
	collector := NewRedis()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestRedisCollectorReturnsReadFileErrors(t *testing.T) {
	expectedErr := errors.New("read failed")
	reader := &redisCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
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

type redisCollectorTestReader struct {
	commandline             configfilesdiscoveryimpl.TargetCommandline
	commandlines            []configfilesdiscoveryimpl.TargetCommandline
	commandlineErr          error
	processCommandlineCalls int
	readFileCalls           []string
	file                    configfilesdiscoveryimpl.ConfigFile
	readFileErr             error
}

func (r *redisCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *redisCollectorTestReader) Close() {}

func (r *redisCollectorTestReader) ReadFile(_ context.Context, path string) (configfilesdiscoveryimpl.ConfigFile, error) {
	r.readFileCalls = append(r.readFileCalls, path)
	if r.readFileErr != nil {
		return configfilesdiscoveryimpl.ConfigFile{}, r.readFileErr
	}
	return r.file, nil
}

func (r *redisCollectorTestReader) ReadEnvVars(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r *redisCollectorTestReader) ReadRuntimeCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	if r.commandlineErr != nil {
		return configfilesdiscoveryimpl.TargetCommandline{}, r.commandlineErr
	}
	return r.commandline, nil
}

func (r *redisCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	r.processCommandlineCalls++
	return r.commandlines
}
