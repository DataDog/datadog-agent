// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisGetConfigPath(t *testing.T) {
	tests := []struct {
		name        string
		commandline TargetCommandline
		wantPath    string
		wantOK      bool
	}{
		{
			name: "official docker custom config",
			commandline: TargetCommandline{
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
			commandline: TargetCommandline{
				Args: []string{"redis-server", "/etc/redis/redis.conf"},
			},
			wantPath: "/etc/redis/redis.conf",
			wantOK:   true,
		},
		{
			name: "redis full config",
			commandline: TargetCommandline{
				Args: []string{"redis-server", "/etc/redis/redis-full.conf"},
			},
			wantPath: "/etc/redis/redis-full.conf",
			wantOK:   true,
		},
		{
			name: "relative path",
			commandline: TargetCommandline{
				Args:       []string{"redis-server", "redis.conf"},
				WorkingDir: "/usr/local/etc/redis",
			},
			wantPath: "/usr/local/etc/redis/redis.conf",
			wantOK:   true,
		},
		{
			name: "relative path with docker default working dir",
			commandline: TargetCommandline{
				Args:       []string{"redis-server", "redis.conf"},
				WorkingDir: "/",
			},
			wantPath: "/redis.conf",
			wantOK:   true,
		},
		{
			name: "arbitrary config filename",
			commandline: TargetCommandline{
				Args: []string{"redis-server", "/usr/local/etc/redis/foo.bar"},
			},
			wantPath: "/usr/local/etc/redis/foo.bar",
			wantOK:   true,
		},
		{
			name: "direct config path without redis server",
			commandline: TargetCommandline{
				Args: []string{"/usr/local/etc/redis/redis.conf"},
			},
		},
		{
			name: "default startup",
			commandline: TargetCommandline{
				Args: []string{"redis-server"},
			},
		},
		{
			name: "flags only",
			commandline: TargetCommandline{
				Args: []string{"redis-server", "--save", "60", "1", "--loglevel", "warning"},
			},
		},
		{
			name: "shell form",
			commandline: TargetCommandline{
				Args: []string{"/bin/sh", "-c", "redis-server /etc/redis/redis.conf"},
			},
			wantPath: "/etc/redis/redis.conf",
			wantOK:   true,
		},
		{
			name: "non redis command",
			commandline: TargetCommandline{
				Args: []string{"nginx", "-c", "/etc/nginx/nginx.conf"},
			},
		},
		{
			name: "relative path without absolute working dir",
			commandline: TargetCommandline{
				Args: []string{"redis-server", "redis.conf"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotOK := redisGetConfigPath(tt.commandline)

			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}

func TestRedisCollectorReadsDetectedConfig(t *testing.T) {
	reader := &redisCollectorTestReader{
		commandline: TargetCommandline{
			Args: []string{"redis-server", "/etc/redis/redis.conf"},
		},
		file: ConfigFile{
			Path:      "/etc/redis/redis.conf",
			Content:   []byte("port 6379\n"),
			Truncated: true,
		},
	}
	collector := newRedisConfigCollector()

	files, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	require.Len(t, files, 1)
	assert.Equal(t, ConfigFile{
		Path:      "/etc/redis/redis.conf",
		Content:   []byte("port 6379\n"),
		Truncated: true,
	}, files[0])
}

func TestRedisCollectorSkipsWhenNoConfigPathIsDetected(t *testing.T) {
	reader := &redisCollectorTestReader{
		commandline: TargetCommandline{
			Args: []string{"redis-server", "--save", "60", "1"},
		},
	}
	collector := newRedisConfigCollector()

	files, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, files)
}

func TestRedisCollectorReturnsCommandlineErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &redisCollectorTestReader{commandlineErr: expectedErr}
	collector := newRedisConfigCollector()

	files, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, files)
}

func TestRedisCollectorReturnsReadFileErrors(t *testing.T) {
	expectedErr := errors.New("read failed")
	reader := &redisCollectorTestReader{
		commandline: TargetCommandline{
			Args: []string{"redis-server", "/etc/redis/redis.conf"},
		},
		readFileErr: expectedErr,
	}
	collector := newRedisConfigCollector()

	files, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []string{"/etc/redis/redis.conf"}, reader.readFileCalls)
	assert.Nil(t, files)
}

type redisCollectorTestReader struct {
	commandline    TargetCommandline
	commandlineErr error
	readFileCalls  []string
	file           ConfigFile
	readFileErr    error
}

func (r *redisCollectorTestReader) Runtime() RuntimeType {
	return RuntimeDocker
}

func (r *redisCollectorTestReader) ReadFile(_ context.Context, path string) (ConfigFile, error) {
	r.readFileCalls = append(r.readFileCalls, path)
	if r.readFileErr != nil {
		return ConfigFile{}, r.readFileErr
	}
	return r.file, nil
}

func (r *redisCollectorTestReader) ReadEnvVars(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r *redisCollectorTestReader) ReadCommandline(context.Context) (TargetCommandline, error) {
	if r.commandlineErr != nil {
		return TargetCommandline{}, r.commandlineErr
	}
	return r.commandline, nil
}
