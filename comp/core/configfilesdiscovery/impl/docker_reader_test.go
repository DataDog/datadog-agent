// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker

package configfilesdiscoveryimpl

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerReaderReportsRuntime(t *testing.T) {
	reader := newDockerConfigReaderWithClient("container-id", &fakeDockerClient{})

	assert.Equal(t, RuntimeDocker, reader.Runtime())
}

func TestNewDockerConfigReaderRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name   string
		target target
	}{
		{
			name:   "non docker runtime",
			target: target{runtime: RuntimeHost, entityID: "container-id"},
		},
		{
			name:   "empty container id",
			target: target{runtime: RuntimeDocker},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := newDockerConfigReader(tt.target, nil)

			require.Error(t, err)
			assert.Nil(t, reader)
		})
	}
}

func TestNewDockerConfigReaderSurfacesDockerClientErrors(t *testing.T) {
	expectedErr := errors.New("docker unavailable")
	newClient := func() (dockerConfigClient, error) {
		return nil, expectedErr
	}

	reader, err := newDockerConfigReaderWithClientFactory(target{runtime: RuntimeDocker, entityID: "container-id"}, nil, newClient)

	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, reader)
}

func TestDockerReaderReadFileReturnsFullContent(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{
			name:    "below limit",
			content: []byte("port 6379\n"),
		},
		{
			name:    "at limit",
			content: bytes.Repeat([]byte("a"), maxConfigFileSize),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeDockerClient{
				copyBody: closeTracker(tarArchive(t, tarEntry{
					name:    "redis.conf",
					mode:    0o600,
					content: tt.content,
				})),
			}
			reader := newDockerConfigReaderWithClient("container-id", client)

			file, err := reader.ReadFile(context.Background(), "/etc/redis/redis.conf")

			require.NoError(t, err)
			assert.Equal(t, "/etc/redis/redis.conf", file.Path)
			assert.Equal(t, tt.content, file.Content)
			assert.False(t, file.Truncated)
			require.Len(t, client.copyCalls, 1)
			assert.Equal(t, dockerCopyCall{
				containerID: "container-id",
				path:        "/etc/redis/redis.conf",
			}, client.copyCalls[0])
		})
	}
}

func TestDockerReaderReadFileTruncatesLargeContent(t *testing.T) {
	content := bytes.Repeat([]byte("a"), maxConfigFileSize+1)
	client := &fakeDockerClient{
		copyBody: closeTracker(tarArchive(t, tarEntry{
			name:    "redis.conf",
			mode:    0o600,
			content: content,
		})),
	}
	reader := newDockerConfigReaderWithClient("container-id", client)

	file, err := reader.ReadFile(context.Background(), "/etc/redis/redis.conf")

	require.NoError(t, err)
	assert.Equal(t, "/etc/redis/redis.conf", file.Path)
	assert.Equal(t, content[:maxConfigFileSize], file.Content)
	assert.True(t, file.Truncated)
}

func TestDockerReaderReadFileClosesArchiveBody(t *testing.T) {
	tests := []struct {
		name    string
		archive []byte
	}{
		{
			name: "success",
			archive: tarArchive(t, tarEntry{
				name:    "redis.conf",
				mode:    0o600,
				content: []byte("port 6379\n"),
			}),
		},
		{
			name: "truncation",
			archive: tarArchive(t, tarEntry{
				name:    "redis.conf",
				mode:    0o600,
				content: bytes.Repeat([]byte("a"), maxConfigFileSize+1),
			}),
		},
		{
			name: "read error",
			archive: tarArchive(t, tarEntry{
				name:     "redis.conf",
				typeflag: tar.TypeSymlink,
				linkname: "target.conf",
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := closeTracker(tt.archive)
			client := &fakeDockerClient{copyBody: body}
			reader := newDockerConfigReaderWithClient("container-id", client)

			_, _ = reader.ReadFile(context.Background(), "/etc/redis/redis.conf")

			assert.True(t, body.closed)
		})
	}
}

func TestDockerReaderReadFileErrors(t *testing.T) {
	copyErr := errors.New("copy failed")
	tests := []struct {
		name          string
		path          string
		copyBody      io.ReadCloser
		copyErr       error
		wantCopyCalls int
		wantErrorIs   error
	}{
		{
			name:          "empty path",
			path:          "",
			wantCopyCalls: 0,
		},
		{
			name:          "relative path",
			path:          "etc/redis/redis.conf",
			wantCopyCalls: 0,
		},
		{
			name:          "parent traversal",
			path:          "/etc/../redis/redis.conf",
			wantCopyCalls: 0,
		},
		{
			name:          "copy error",
			path:          "/etc/redis/redis.conf",
			copyErr:       copyErr,
			wantCopyCalls: 1,
			wantErrorIs:   copyErr,
		},
		{
			name: "directory",
			path: "/etc/redis",
			copyBody: closeTracker(tarArchive(t, tarEntry{
				name:     "redis",
				typeflag: tar.TypeDir,
				mode:     0o755,
			})),
			wantCopyCalls: 1,
		},
		{
			name:          "empty archive",
			path:          "/etc/redis/redis.conf",
			copyBody:      closeTracker(tarArchive(t)),
			wantCopyCalls: 1,
		},
		{
			name:          "invalid archive",
			path:          "/etc/redis/redis.conf",
			copyBody:      closeTracker([]byte("not a tar archive")),
			wantCopyCalls: 1,
		},
		{
			name: "symlink",
			path: "/etc/redis/redis.conf",
			copyBody: closeTracker(tarArchive(t, tarEntry{
				name:     "redis.conf",
				typeflag: tar.TypeSymlink,
				linkname: "target.conf",
			})),
			wantCopyCalls: 1,
		},
		{
			name: "ambiguous archive",
			path: "/etc/redis/redis.conf",
			copyBody: closeTracker(tarArchive(t,
				tarEntry{name: "redis.conf", mode: 0o600, content: []byte("first")},
				tarEntry{name: "redis.conf.bak", mode: 0o600, content: []byte("second")},
			)),
			wantCopyCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeDockerClient{
				copyBody: tt.copyBody,
				copyErr:  tt.copyErr,
			}
			reader := newDockerConfigReaderWithClient("container-id", client)

			file, err := reader.ReadFile(context.Background(), tt.path)

			require.Error(t, err)
			assert.Empty(t, file)
			if tt.wantErrorIs != nil {
				assert.ErrorIs(t, err, tt.wantErrorIs)
			}
			assert.Len(t, client.copyCalls, tt.wantCopyCalls)
		})
	}
}

func TestDockerReaderReadEnvVarsSkipsInspectForEmptyWhitelist(t *testing.T) {
	client := &fakeDockerClient{}
	reader := newDockerConfigReaderWithClient("container-id", client)

	env, err := reader.ReadEnvVars(context.Background(), nil)

	require.NoError(t, err)
	assert.Empty(t, env)
	assert.Empty(t, client.getEnvCalls)
}

func TestDockerReaderReadEnvVarsFiltersRequestedNames(t *testing.T) {
	client := &fakeDockerClient{
		env: []string{
			"REDIS_PASSWORD=first",
			"MALFORMED",
			"WITH_EQUALS=a=b=c",
			"EMPTY=",
			"REDIS_PASSWORD=last",
			"UNREQUESTED=value",
		},
	}
	reader := newDockerConfigReaderWithClient("container-id", client)

	env, err := reader.ReadEnvVars(context.Background(), []string{
		"REDIS_PASSWORD",
		"WITH_EQUALS",
		"EMPTY",
		"MISSING",
	})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"REDIS_PASSWORD": "last",
		"WITH_EQUALS":    "a=b=c",
		"EMPTY":          "",
	}, env)
	assert.Equal(t, []string{"container-id"}, client.getEnvCalls)
}

func TestDockerReaderReadEnvVarsSurfacesGetEnvErrors(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	client := &fakeDockerClient{getEnvErr: expectedErr}
	reader := newDockerConfigReaderWithClient("container-id", client)

	env, err := reader.ReadEnvVars(context.Background(), []string{"REDIS_PASSWORD"})

	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, env)
	assert.Equal(t, []string{"container-id"}, client.getEnvCalls)
}

func TestDockerReaderReadRuntimeCommandlineReturnsTargetCommandline(t *testing.T) {
	client := &fakeDockerClient{
		commandPath: "/usr/local/bin/redis-server",
		commandArgs: []string{
			"/usr/local/etc/redis/redis.conf",
			"--loglevel",
			"warning",
		},
		workingDir: "/usr/local/etc/redis",
	}
	reader := newDockerConfigReaderWithClient("container-id", client)

	commandline, err := reader.ReadRuntimeCommandline(context.Background())

	require.NoError(t, err)
	assert.Equal(t, TargetCommandline{
		Args: []string{
			"/usr/local/bin/redis-server",
			"/usr/local/etc/redis/redis.conf",
			"--loglevel",
			"warning",
		},
		WorkingDir: "/usr/local/etc/redis",
	}, commandline)
	assert.Equal(t, []string{"container-id"}, client.getCommandlineCalls)
}

func TestDockerReaderReadRuntimeCommandlineAllowsEmptyCommandPath(t *testing.T) {
	client := &fakeDockerClient{
		commandArgs: []string{"redis-server"},
	}
	reader := newDockerConfigReaderWithClient("container-id", client)

	commandline, err := reader.ReadRuntimeCommandline(context.Background())

	require.NoError(t, err)
	assert.Equal(t, TargetCommandline{Args: []string{"redis-server"}, WorkingDir: "/"}, commandline)
}

func TestDockerReaderReadRuntimeCommandlineDefaultsEmptyWorkingDirToRoot(t *testing.T) {
	client := &fakeDockerClient{
		commandPath: "redis-server",
		commandArgs: []string{
			"redis.conf",
		},
	}
	reader := newDockerConfigReaderWithClient("container-id", client)

	commandline, err := reader.ReadRuntimeCommandline(context.Background())

	require.NoError(t, err)
	assert.Equal(t, TargetCommandline{
		Args:       []string{"redis-server", "redis.conf"},
		WorkingDir: "/",
	}, commandline)
}

func TestDockerReaderReadRuntimeCommandlineSurfacesGetCommandlineErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	client := &fakeDockerClient{getCommandlineErr: expectedErr}
	reader := newDockerConfigReaderWithClient("container-id", client)

	commandline, err := reader.ReadRuntimeCommandline(context.Background())

	require.ErrorIs(t, err, expectedErr)
	assert.Empty(t, commandline)
	assert.Equal(t, []string{"container-id"}, client.getCommandlineCalls)
}

type fakeDockerClient struct {
	copyCalls           []dockerCopyCall
	copyBody            io.ReadCloser
	copyErr             error
	getEnvCalls         []string
	env                 []string
	getEnvErr           error
	getCommandlineCalls []string
	commandPath         string
	commandArgs         []string
	workingDir          string
	getCommandlineErr   error
}

type dockerCopyCall struct {
	containerID string
	path        string
}

func (c *fakeDockerClient) getFile(_ context.Context, containerID string, path string) (io.ReadCloser, error) {
	c.copyCalls = append(c.copyCalls, dockerCopyCall{containerID: containerID, path: path})
	if c.copyErr != nil {
		return nil, c.copyErr
	}
	return c.copyBody, nil
}

func (c *fakeDockerClient) getEnv(_ context.Context, containerID string) ([]string, error) {
	c.getEnvCalls = append(c.getEnvCalls, containerID)
	if c.getEnvErr != nil {
		return nil, c.getEnvErr
	}
	return c.env, nil
}

func (c *fakeDockerClient) getCommandline(_ context.Context, containerID string) (TargetCommandline, error) {
	c.getCommandlineCalls = append(c.getCommandlineCalls, containerID)
	if c.getCommandlineErr != nil {
		return TargetCommandline{}, c.getCommandlineErr
	}
	return targetCommandlineFromDockerConfig(c.commandPath, c.commandArgs, c.workingDir), nil
}

type trackingReadCloser struct {
	*bytes.Reader
	closed bool
}

func closeTracker(content []byte) *trackingReadCloser {
	return &trackingReadCloser{Reader: bytes.NewReader(content)}
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

type tarEntry struct {
	name     string
	typeflag byte
	mode     int64
	linkname string
	content  []byte
}

func tarArchive(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		mode := entry.mode
		if mode == 0 {
			mode = 0o600
		}
		header := &tar.Header{
			Name:     entry.name,
			Typeflag: typeflag,
			Mode:     mode,
			Size:     int64(len(entry.content)),
			Linkname: entry.linkname,
		}
		require.NoError(t, tw.WriteHeader(header))
		if len(entry.content) > 0 {
			_, err := io.Copy(tw, bytes.NewReader(entry.content))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())

	return buf.Bytes()
}
