// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// A Docker client that is connected to an [docker.Deamon].
//
// [docker.Deamon]: https://pkg.go.dev/github.com/DataDog/datadog-agent/test/e2e-framework@main/components/datadog/agent/docker#Deamon
type Docker struct {
	t        *testing.T
	client   *client.Client
	scrubber *scrubber.Scrubber
}

// NewDocker creates a new instance of Docker
// NOTE: docker+ssh does not support password protected SSH keys.
func NewDocker(t *testing.T, dockerOutput docker.ManagerOutput) (*Docker, error) {
	deamonURL := fmt.Sprintf("ssh://%v@%v", dockerOutput.Host.Username, dockerOutput.Host.Address)

	sshOpts := []string{"-o", "StrictHostKeyChecking no"}

	privateKeyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.StoreKey(dockerOutput.Host.CloudProvider+parameters.PrivateKeyPathSuffix), "")
	if err != nil {
		return nil, err
	}
	if privateKeyPath != "" {
		sshOpts = append(sshOpts, "-i", privateKeyPath)
	}

	helper, err := connhelper.GetConnectionHelperWithSSHOpts(deamonURL, sshOpts)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to docker %v: %w", deamonURL, err)
	}

	opts := []client.Opt{
		client.WithDialContext(helper.Dialer),
		client.WithAPIVersionNegotiation(),
	}

	client, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("cannot create docker client: %w", err)
	}

	return &Docker{
		t:        t,
		client:   client,
		scrubber: scrubber.NewWithDefaults(),
	}, nil
}

// ExecuteCommand executes a command on containerName and returns the output.
func (docker *Docker) ExecuteCommand(containerName string, commands ...string) string {
	output, err := docker.ExecuteCommandWithErr(containerName, commands...)
	require.NoErrorf(docker.t, err, "%v: %v", output, err)
	return output
}

// ExecuteCommandWithErr executes a command on containerName and returns the output and an error.
func (docker *Docker) ExecuteCommandWithErr(containerName string, commands ...string) (string, error) {
	output, errOutput, err := docker.ExecuteCommandStdoutStdErr(containerName, commands...)
	if len(errOutput) != 0 {
		output += " " + errOutput
	}
	return output, err
}

// ExecuteCommandStdoutStdErr executes a command on containerName and returns the output, the error output and an error.
func (docker *Docker) ExecuteCommandStdoutStdErr(containerName string, commands ...string) (stdout string, stderr string, err error) {
	cmd := strings.Join(commands, " ")
	scrubbedCommand := docker.scrubber.ScrubLine(cmd) // scrub the command in case it contains secrets
	docker.t.Logf("Executing command `%s`", scrubbedCommand)

	context := context.Background()
	execConfig := container.ExecOptions{Cmd: commands, AttachStderr: true, AttachStdout: true}
	execCreateResp, err := docker.client.ContainerExecCreate(context, containerName, execConfig)
	require.NoError(docker.t, err)

	execAttachResp, err := docker.client.ContainerExecAttach(context, execCreateResp.ID, container.ExecAttachOptions{})
	require.NoError(docker.t, err)
	defer execAttachResp.Close()

	var outBuf, errBuf bytes.Buffer
	// Use stdcopy.StdCopy to remove prefix for stdout and stderr
	// See https://stackoverflow.com/questions/52774830/docker-exec-command-from-golang-api for additional context
	_, err = stdcopy.StdCopy(&outBuf, &errBuf, execAttachResp.Reader)
	require.NoError(docker.t, err)

	execInspectResp, err := docker.client.ContainerExecInspect(context, execCreateResp.ID)
	require.NoError(docker.t, err)

	stdout = outBuf.String()
	stderr = errBuf.String()

	if execInspectResp.ExitCode != 0 {
		return "", "", fmt.Errorf("error when running command %v on container %v:\n   exit code: %d\n   stdout: %v\n   stderr: %v", commands, containerName, execInspectResp.ExitCode, stdout, stderr)
	}

	return stdout, suppressGoCoverWarning(stderr), err
}

// ListContainers returns a list of container names.
func (docker *Docker) ListContainers() ([]string, error) {
	containersMap, err := docker.getContainerIDsByName()
	if err != nil {
		return nil, err
	}
	containerNames := make([]string, 0, len(containersMap))
	for name := range containersMap {
		containerNames = append(containerNames, name)
	}
	return containerNames, nil
}

func (docker *Docker) getContainerIDsByName() (map[string]string, error) {
	containersMap := make(map[string]string)
	containers, err := docker.client.ContainerList(context.Background(), container.ListOptions{All: true})
	if err != nil {
		return containersMap, err
	}
	for _, container := range containers {
		for _, name := range container.Names {
			// remove leading /
			name = strings.TrimPrefix(name, "/")
			containersMap[name] = container.ID
		}
	}
	return containersMap, nil
}

// DownloadFile downloads a file or directory from the container to the local filesystem.
func (docker *Docker) DownloadFile(containerName, containerPath, localPath string) error {
	docker.t.Logf("Downloading from container %s:%s to local path %s", containerName, containerPath, localPath)

	ctx := context.Background()
	reader, _, err := docker.client.CopyFromContainer(ctx, containerName, containerPath)
	if err != nil {
		return fmt.Errorf("failed to copy from container %s:%s: %w", containerName, containerPath, err)
	}
	defer reader.Close()

	tarReader := tar.NewReader(reader)

	// Process all entries in the tar archive
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Get the base name of the container path to strip it from entry names
		baseName := filepath.Base(containerPath)
		entryName := strings.TrimPrefix(header.Name, "./")

		// For single files, use the localPath directly
		// For directories, strip the top-level directory name and place contents under localPath
		var target string
		if header.Typeflag == tar.TypeReg && entryName == baseName {
			// Single file case
			target = localPath
		} else {
			// Directory case - strip the base directory name
			if entryName == baseName || entryName == baseName+"/" {
				// Skip the root directory entry itself
				continue
			}
			if after, ok := strings.CutPrefix(entryName, baseName+"/"); ok {
				entryName = after
			}
			target = filepath.Join(localPath, entryName)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", target, err)
			}
			docker.t.Logf("Created directory: %s", target)

		case tar.TypeReg:
			// Ensure parent directory exists
			dir := filepath.Dir(target)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create parent directory %s: %w", dir, err)
			}

			outFile, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}

			_, err = io.Copy(outFile, tarReader)
			closeErr := outFile.Close()
			if err != nil {
				return fmt.Errorf("failed to write file contents to %s: %w", target, err)
			}
			if closeErr != nil {
				return fmt.Errorf("failed to close file %s: %w", target, closeErr)
			}

			docker.t.Logf("Created file: %s", target)
		}
	}

	docker.t.Logf("Successfully downloaded to %s", localPath)
	return nil
}
