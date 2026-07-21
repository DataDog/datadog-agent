// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build cri && containerd

package configfilesdiscoveryimpl

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	containerdoci "github.com/containerd/containerd/v2/pkg/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKubernetesReaderReportsRuntime(t *testing.T) {
	reader := newKubernetesConfigReaderWithClient("container-id", &fakeKubernetesClient{})

	assert.Equal(t, RuntimeKubernetes, reader.Runtime())
}

func TestKubernetesReaderCloseClosesClient(t *testing.T) {
	client := &fakeKubernetesClient{}
	reader := newKubernetesConfigReaderWithClient("container-id", client)

	reader.Close()

	assert.Equal(t, 1, client.closeCalls)
}

func TestNewKubernetesConfigReaderRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name   string
		target target
	}{
		{
			name:   "non kubernetes runtime",
			target: target{runtime: RuntimeDocker, entityID: "container-id"},
		},
		{
			name:   "empty container id",
			target: target{runtime: RuntimeKubernetes},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := newKubernetesConfigReader(tt.target)

			require.Error(t, err)
			assert.Nil(t, reader)
		})
	}
}

func TestNewKubernetesConfigReaderSurfacesClientErrors(t *testing.T) {
	expectedErr := errors.New("cri unavailable")
	oldNewKubernetesConfigClient := newKubernetesConfigClient
	newKubernetesConfigClient = func() (kubernetesConfigClient, error) {
		return nil, expectedErr
	}
	t.Cleanup(func() {
		newKubernetesConfigClient = oldNewKubernetesConfigClient
	})

	reader, err := newKubernetesConfigReader(target{runtime: RuntimeKubernetes, entityID: "container-id"})

	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, reader)
}

func TestKubernetesReaderReadFileReturnsFullContent(t *testing.T) {
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
			client := &fakeKubernetesClient{stdout: tt.content}
			reader := newKubernetesConfigReaderWithClient("container-id", client)

			file, err := reader.ReadFile(context.Background(), "/etc/redis/redis.conf")

			require.NoError(t, err)
			assert.Equal(t, "/etc/redis/redis.conf", file.Path)
			assert.Equal(t, tt.content, file.Content)
			assert.False(t, file.Truncated)
			require.Len(t, client.execCalls, 1)
			assert.Equal(t, kubernetesExecCall{
				containerID: "container-id",
				cmd:         []string{"head", "-c", strconv.Itoa(maxConfigFileSize + 1), "/etc/redis/redis.conf"},
				timeout:     kubernetesReadFileTimeout,
			}, client.execCalls[0])
		})
	}
}

func TestKubernetesReaderReadFileTruncatesLargeContent(t *testing.T) {
	content := bytes.Repeat([]byte("a"), maxConfigFileSize+1)
	client := &fakeKubernetesClient{stdout: content}
	reader := newKubernetesConfigReaderWithClient("container-id", client)

	file, err := reader.ReadFile(context.Background(), "/etc/redis/redis.conf")

	require.NoError(t, err)
	assert.Equal(t, "/etc/redis/redis.conf", file.Path)
	assert.Equal(t, content[:maxConfigFileSize], file.Content)
	assert.True(t, file.Truncated)
	require.Len(t, client.execCalls, 1)
	assert.Equal(t, []string{"head", "-c", strconv.Itoa(maxConfigFileSize + 1), "/etc/redis/redis.conf"}, client.execCalls[0].cmd)
}

func TestKubernetesReaderReadFileErrors(t *testing.T) {
	execErr := errors.New("runtime unavailable")
	tests := []struct {
		name          string
		path          string
		stdout        []byte
		stderr        []byte
		exitCode      int32
		execErr       error
		wantExecCalls int
		wantErrorIs   error
		wantContains  string
	}{
		{
			name:          "empty path",
			path:          "",
			wantExecCalls: 0,
			wantContains:  "empty config file path",
		},
		{
			name:          "relative path",
			path:          "etc/redis/redis.conf",
			wantExecCalls: 0,
			wantContains:  "is not absolute",
		},
		{
			name:          "parent traversal",
			path:          "/etc/../redis/redis.conf",
			wantExecCalls: 0,
			wantContains:  "contains parent traversal",
		},
		{
			name:          "exec error",
			path:          "/etc/redis/redis.conf",
			execErr:       execErr,
			wantExecCalls: 1,
			wantErrorIs:   execErr,
		},
		{
			name:          "nonzero exit with stderr",
			path:          "/etc/redis/redis.conf",
			stderr:        []byte("cat: /etc/redis/redis.conf: No such file"),
			exitCode:      1,
			wantExecCalls: 1,
			wantContains:  "No such file",
		},
		{
			name:          "nonzero exit without stderr",
			path:          "/etc/redis/redis.conf",
			exitCode:      2,
			wantExecCalls: 1,
			wantContains:  "exited with code 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeKubernetesClient{
				stdout:   tt.stdout,
				stderr:   tt.stderr,
				exitCode: tt.exitCode,
				execErr:  tt.execErr,
			}
			reader := newKubernetesConfigReaderWithClient("container-id", client)

			file, err := reader.ReadFile(context.Background(), tt.path)

			require.Error(t, err)
			assert.Empty(t, file)
			if tt.wantErrorIs != nil {
				assert.ErrorIs(t, err, tt.wantErrorIs)
			}
			if tt.wantContains != "" {
				assert.ErrorContains(t, err, tt.wantContains)
			}
			assert.Len(t, client.execCalls, tt.wantExecCalls)
		})
	}
}

func TestKubernetesReaderReadEnvVarsSkipsSpecForEmptyWhitelist(t *testing.T) {
	client := &fakeKubernetesClient{}
	reader := newKubernetesConfigReaderWithClient("container-id", client)

	env, err := reader.ReadEnvVars(context.Background(), nil)

	require.NoError(t, err)
	assert.Empty(t, env)
	assert.Empty(t, client.specCalls)
}

func TestKubernetesReaderReadEnvVarsFiltersRequestedNames(t *testing.T) {
	client := &fakeKubernetesClient{
		spec: &containerdoci.Spec{
			Process: &specs.Process{
				Env: []string{
					"REDIS_PASSWORD=first",
					"MALFORMED",
					"WITH_EQUALS=a=b=c",
					"EMPTY=",
					"REDIS_PASSWORD=last",
					"UNREQUESTED=value",
				},
			},
		},
	}
	reader := newKubernetesConfigReaderWithClient("container-id", client)

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
	assert.Equal(t, []string{"container-id"}, client.specCalls)
}

func TestKubernetesReaderReadEnvVarsSurfacesSpecErrors(t *testing.T) {
	expectedErr := errors.New("spec unavailable")
	client := &fakeKubernetesClient{specErr: expectedErr}
	reader := newKubernetesConfigReaderWithClient("container-id", client)

	env, err := reader.ReadEnvVars(context.Background(), []string{"REDIS_PASSWORD"})

	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, env)
	assert.Equal(t, []string{"container-id"}, client.specCalls)
}

func TestKubernetesReaderReadCommandlineReturnsTargetCommandline(t *testing.T) {
	client := &fakeKubernetesClient{
		spec: &containerdoci.Spec{
			Process: &specs.Process{
				Args: []string{
					"/usr/local/bin/redis-server",
					"/usr/local/etc/redis/redis.conf",
					"--loglevel",
					"warning",
				},
				Cwd: "/usr/local/etc/redis",
			},
		},
	}
	reader := newKubernetesConfigReaderWithClient("container-id", client)

	commandline, err := reader.ReadCommandline(context.Background())

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
	assert.Equal(t, []string{"container-id"}, client.specCalls)
}

func TestKubernetesReaderReadCommandlineDefaultsEmptyWorkingDirToRoot(t *testing.T) {
	client := &fakeKubernetesClient{
		spec: &containerdoci.Spec{
			Process: &specs.Process{
				Args: []string{"redis-server", "redis.conf"},
			},
		},
	}
	reader := newKubernetesConfigReaderWithClient("container-id", client)

	commandline, err := reader.ReadCommandline(context.Background())

	require.NoError(t, err)
	assert.Equal(t, TargetCommandline{
		Args:       []string{"redis-server", "redis.conf"},
		WorkingDir: "/",
	}, commandline)
}

func TestKubernetesReaderReadCommandlineHandlesMissingProcess(t *testing.T) {
	client := &fakeKubernetesClient{spec: &containerdoci.Spec{}}
	reader := newKubernetesConfigReaderWithClient("container-id", client)

	commandline, err := reader.ReadCommandline(context.Background())

	require.NoError(t, err)
	assert.Equal(t, TargetCommandline{WorkingDir: "/"}, commandline)
}

func TestKubernetesReaderReadCommandlineSurfacesSpecErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	client := &fakeKubernetesClient{specErr: expectedErr}
	reader := newKubernetesConfigReaderWithClient("container-id", client)

	commandline, err := reader.ReadCommandline(context.Background())

	require.ErrorIs(t, err, expectedErr)
	assert.Empty(t, commandline)
	assert.Equal(t, []string{"container-id"}, client.specCalls)
}

type fakeKubernetesClient struct {
	execCalls  []kubernetesExecCall
	stdout     []byte
	stderr     []byte
	exitCode   int32
	execErr    error
	specCalls  []string
	spec       *containerdoci.Spec
	specErr    error
	closeCalls int
}

type kubernetesExecCall struct {
	containerID string
	cmd         []string
	timeout     time.Duration
}

func (c *fakeKubernetesClient) execSync(_ context.Context, containerID string, cmd []string, timeout time.Duration) ([]byte, []byte, int32, error) {
	c.execCalls = append(c.execCalls, kubernetesExecCall{
		containerID: containerID,
		cmd:         append([]string(nil), cmd...),
		timeout:     timeout,
	})
	if c.execErr != nil {
		return nil, nil, 0, c.execErr
	}
	return c.stdout, c.stderr, c.exitCode, nil
}

func (c *fakeKubernetesClient) containerSpec(_ context.Context, containerID string) (*containerdoci.Spec, error) {
	c.specCalls = append(c.specCalls, containerID)
	if c.specErr != nil {
		return nil, c.specErr
	}
	return c.spec, nil
}

func (c *fakeKubernetesClient) close() {
	c.closeCalls++
}
