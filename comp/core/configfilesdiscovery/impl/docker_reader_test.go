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
	reader := &dockerConfigReader{containerID: "container-id", client: &fakeDockerClient{}}

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
			reader := &dockerConfigReader{containerID: "container-id", client: client}

			file, err := reader.ReadFile(context.Background(), verifyTestConfigFilePath(t, "/etc/redis/redis.conf"))

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
	reader := &dockerConfigReader{containerID: "container-id", client: client}

	file, err := reader.ReadFile(context.Background(), verifyTestConfigFilePath(t, "/etc/redis/redis.conf"))

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
			reader := &dockerConfigReader{containerID: "container-id", client: client}

			_, _ = reader.ReadFile(context.Background(), verifyTestConfigFilePath(t, "/etc/redis/redis.conf"))

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
			reader := &dockerConfigReader{containerID: "container-id", client: client}

			file, err := reader.ReadFile(context.Background(), verifyTestConfigFilePath(t, tt.path))

			require.Error(t, err)
			assert.Empty(t, file)
			if tt.wantErrorIs != nil {
				assert.ErrorIs(t, err, tt.wantErrorIs)
			}
			assert.Len(t, client.copyCalls, tt.wantCopyCalls)
		})
	}
}

func TestDockerReaderFindFiles(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		maxMatches  int
		archive     []byte
		wantPaths   []string
		wantLimited bool
		wantCopy    string
	}{
		{
			name:       "literal file",
			pattern:    "/etc/redis/redis.conf",
			maxMatches: 2,
			archive: tarArchive(t, tarEntry{
				name:    "redis.conf",
				content: []byte("port 6379\n"),
			}),
			wantPaths: []string{"/etc/redis/redis.conf"},
			wantCopy:  "/etc/redis/redis.conf",
		},
		{
			name:       "wildcard sorts limits and rejects non regular files",
			pattern:    "/etc/redis/conf.d/*.conf",
			maxMatches: 2,
			archive: tarArchive(t,
				tarEntry{name: "conf.d", typeflag: tar.TypeDir},
				tarEntry{name: "etc/redis/conf.d/c.conf", content: []byte("c")},
				tarEntry{name: "conf.d/link.conf", typeflag: tar.TypeSymlink, linkname: "a.conf"},
				tarEntry{name: "conf.d/a.conf", content: []byte("a")},
				tarEntry{name: "conf.d/b.conf", content: []byte("b")},
				tarEntry{name: "conf.d/readme.txt", content: []byte("ignored")},
			),
			wantPaths:   []string{"/etc/redis/conf.d/a.conf", "/etc/redis/conf.d/b.conf"},
			wantLimited: true,
			wantCopy:    "/etc/redis/conf.d",
		},
		{
			name:       "literal symlink is rejected",
			pattern:    "/etc/redis/link.conf",
			maxMatches: 1,
			archive: tarArchive(t, tarEntry{
				name:     "link.conf",
				typeflag: tar.TypeSymlink,
				linkname: "redis.conf",
			}),
			wantCopy: "/etc/redis/link.conf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := closeTracker(tt.archive)
			client := &fakeDockerClient{copyBody: body}
			reader := &dockerConfigReader{containerID: "container-id", client: client}

			paths, limited, err := reader.FindFiles(context.Background(), verifyTestConfigFilePattern(t, tt.pattern), tt.maxMatches, matchTestFilePattern(tt.pattern))

			require.NoError(t, err)
			assert.Equal(t, tt.wantPaths, verifiedConfigFilePathStrings(paths))
			assert.Equal(t, tt.wantLimited, limited)
			assert.Equal(t, []dockerCopyCall{{containerID: "container-id", path: tt.wantCopy}}, client.copyCalls)
			assert.True(t, body.closed)
		})
	}
}

func TestDockerReaderFindFilesErrors(t *testing.T) {
	expectedErr := errors.New("copy failed")
	tests := []struct {
		name          string
		pattern       string
		maxMatches    int
		copyBody      io.ReadCloser
		copyErr       error
		wantCopyCalls int
		wantErrorIs   error
	}{
		{name: "non positive limit", pattern: "/etc/redis/*.conf", maxMatches: 0},
		{name: "copy error", pattern: "/etc/redis/*.conf", maxMatches: 1, copyErr: expectedErr, wantCopyCalls: 1, wantErrorIs: expectedErr},
		{name: "cancellation", pattern: "/etc/redis/*.conf", maxMatches: 1, copyErr: context.Canceled, wantCopyCalls: 1, wantErrorIs: context.Canceled},
		{name: "archive error", pattern: "/etc/redis/*.conf", maxMatches: 1, copyBody: closeTracker([]byte("not a tar archive")), wantCopyCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeDockerClient{copyBody: tt.copyBody, copyErr: tt.copyErr}
			reader := &dockerConfigReader{containerID: "container-id", client: client}

			paths, limited, err := reader.FindFiles(context.Background(), verifyTestConfigFilePattern(t, tt.pattern), tt.maxMatches, matchTestFilePattern(tt.pattern))

			require.Error(t, err)
			assert.Nil(t, paths)
			assert.False(t, limited)
			assert.Len(t, client.copyCalls, tt.wantCopyCalls)
			if tt.wantErrorIs != nil {
				assert.ErrorIs(t, err, tt.wantErrorIs)
			}
		})
	}
}

func TestDockerReaderReadEnvVarsSkipsInspectForNilPredicate(t *testing.T) {
	client := &fakeDockerClient{}
	reader := &dockerConfigReader{containerID: "container-id", client: client}

	env, err := reader.ReadEnvVars(context.Background(), nil)

	require.NoError(t, err)
	assert.Empty(t, env)
	assert.Empty(t, client.getEnvCalls)
}

func TestDockerReaderReadEnvVarsFiltersWithPredicate(t *testing.T) {
	client := &fakeDockerClient{
		env: []string{"KAFKA_NODE_ID=1"},
	}
	reader := &dockerConfigReader{containerID: "container-id", client: client}

	env, err := reader.ReadEnvVars(context.Background(), func(name string) bool {
		return name == "KAFKA_NODE_ID"
	})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"KAFKA_NODE_ID": "1",
	}, env)
	assert.Equal(t, []string{"container-id"}, client.getEnvCalls)
}

func TestDockerReaderReadEnvVarsSurfacesGetEnvErrors(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	client := &fakeDockerClient{getEnvErr: expectedErr}
	reader := &dockerConfigReader{containerID: "container-id", client: client}

	env, err := reader.ReadEnvVars(context.Background(), func(name string) bool {
		return name == "KAFKA_NODE_ID"
	})

	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, env)
	assert.Equal(t, []string{"container-id"}, client.getEnvCalls)
}

func TestDockerReaderReadRuntimeCommandline(t *testing.T) {
	tests := []struct {
		name        string
		commandPath string
		commandArgs []string
		workingDir  string
		want        TargetCommandline
	}{
		{
			name:        "command and working directory",
			commandPath: "/usr/local/bin/redis-server",
			commandArgs: []string{
				"/usr/local/etc/redis/redis.conf",
				"--loglevel",
				"warning",
			},
			workingDir: "/usr/local/etc/redis",
			want: TargetCommandline{
				Args: []string{
					"/usr/local/bin/redis-server",
					"/usr/local/etc/redis/redis.conf",
					"--loglevel",
					"warning",
				},
				WorkingDir: "/usr/local/etc/redis",
			},
		},
		{
			name:        "empty command path",
			commandArgs: []string{"redis-server"},
			want:        TargetCommandline{Args: []string{"redis-server"}, WorkingDir: "/"},
		},
		{
			name:        "empty working directory",
			commandPath: "redis-server",
			commandArgs: []string{"redis.conf"},
			want: TargetCommandline{
				Args:       []string{"redis-server", "redis.conf"},
				WorkingDir: "/",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeDockerClient{
				commandPath: tt.commandPath,
				commandArgs: tt.commandArgs,
				workingDir:  tt.workingDir,
			}
			reader := &dockerConfigReader{containerID: "container-id", client: client}

			commandline, err := reader.ReadRuntimeCommandline(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.want, commandline)
			assert.Equal(t, []string{"container-id"}, client.getCommandlineCalls)
		})
	}
}

func TestDockerReaderReadRuntimeCommandlineSurfacesGetCommandlineErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	client := &fakeDockerClient{getCommandlineErr: expectedErr}
	reader := &dockerConfigReader{containerID: "container-id", client: client}

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
