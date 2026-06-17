// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker

package configfilesdiscoveryimpl

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	dockerutil "github.com/DataDog/datadog-agent/pkg/util/docker"
)

// dockerConfigClient is a narrow Docker interface; reader tests mock it so tar
// decoding, env filtering, and command-line extraction are tested without a
// Docker daemon.
type dockerConfigClient interface {
	getFile(context.Context, string, string) (io.ReadCloser, error)
	getEnv(context.Context, string) ([]string, error)
	getCommandline(context.Context, string) (TargetCommandline, error)
}

func newDockerConfigClient() (dockerConfigClient, error) {
	util, err := dockerutil.GetDockerUtil()
	if err != nil {
		return nil, err
	}
	return dockerUtilConfigClient{util: util}, nil
}

type dockerConfigReader struct {
	containerID string
	client      dockerConfigClient
}

func newDockerConfigReader(t target) (ConfigReader, error) {
	return newDockerConfigReaderWithClientFactory(t, newDockerConfigClient)
}

func newDockerConfigReaderWithClientFactory(t target, newClient func() (dockerConfigClient, error)) (ConfigReader, error) {
	if t.runtime != RuntimeDocker {
		return nil, fmt.Errorf("unsupported runtime %q", t.runtime)
	}
	if t.entityID == "" {
		return nil, errors.New("empty docker container id")
	}

	client, err := newClient()
	if err != nil {
		return nil, err
	}

	return newDockerConfigReaderWithClient(t.entityID, client), nil
}

func newDockerConfigReaderWithClient(containerID string, client dockerConfigClient) ConfigReader {
	return &dockerConfigReader{
		containerID: containerID,
		client:      client,
	}
}

func (r *dockerConfigReader) Runtime() RuntimeType {
	return RuntimeDocker
}

func (r *dockerConfigReader) Close() {}

func (r *dockerConfigReader) ReadFile(ctx context.Context, filePath string) (ConfigFile, error) {
	cleanPath, err := cleanContainerFilePath(filePath)
	if err != nil {
		return ConfigFile{}, err
	}

	body, err := r.client.getFile(ctx, r.containerID, cleanPath)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("copy config file from docker container: %w", err)
	}
	defer body.Close()

	return readConfigFileFromDockerArchive(body, cleanPath)
}

func (r *dockerConfigReader) ReadEnvVars(ctx context.Context, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}

	envEntries, err := r.client.getEnv(ctx, r.containerID)
	if err != nil {
		return nil, fmt.Errorf("get docker container env: %w", err)
	}

	return filterEnvVars(envEntries, names), nil
}

func (r *dockerConfigReader) ReadCommandline(ctx context.Context) (TargetCommandline, error) {
	commandline, err := r.client.getCommandline(ctx, r.containerID)
	if err != nil {
		return TargetCommandline{}, fmt.Errorf("get docker container command line: %w", err)
	}

	return commandline, nil
}

func readConfigFileFromDockerArchive(r io.Reader, requestedPath string) (ConfigFile, error) {
	content, truncated, err := readRegularFileFromTar(r, requestedPath)
	if err != nil {
		return ConfigFile{}, err
	}

	return ConfigFile{
		Path:      requestedPath,
		Content:   content,
		Truncated: truncated,
	}, nil
}

func readRegularFileFromTar(r io.Reader, requestedPath string) ([]byte, bool, error) {
	tr := tar.NewReader(r)

	header, err := tr.Next()
	if err == io.EOF {
		return nil, false, errors.New("empty docker archive")
	}
	if err != nil {
		return nil, false, fmt.Errorf("read docker archive: %w", err)
	}

	if !isRegularTarEntry(header) {
		return nil, false, fmt.Errorf("docker archive entry %q is not a regular file", header.Name)
	}
	if !matchesRequestedPath(header.Name, requestedPath) {
		return nil, false, fmt.Errorf("docker archive entry %q does not match requested path %q", header.Name, requestedPath)
	}

	content, truncated, err := readLimitedFileContent(tr, maxConfigFileSize)
	if err != nil {
		return nil, false, fmt.Errorf("read docker archive entry %q: %w", header.Name, err)
	}
	if truncated {
		return content, true, nil
	}

	next, err := tr.Next()
	if err != io.EOF {
		if err != nil {
			return nil, false, fmt.Errorf("read docker archive: %w", err)
		}
		return nil, false, fmt.Errorf("ambiguous docker archive includes multiple entries: %q and %q", header.Name, next.Name)
	}

	return content, false, nil
}

func isRegularTarEntry(header *tar.Header) bool {
	// tar.Reader normalizes the legacy NUL regular-file marker to TypeReg.
	return header.Typeflag == tar.TypeReg
}

func matchesRequestedPath(entryName string, requestedPath string) bool {
	entryPath := cleanTarPath(entryName)
	requested := cleanTarPath(requestedPath)
	return entryPath == requested || entryPath == path.Base(requested)
}

func cleanTarPath(filePath string) string {
	return strings.TrimPrefix(path.Clean(filePath), "/")
}

type dockerUtilConfigClient struct {
	util *dockerutil.DockerUtil
}

func (c dockerUtilConfigClient) getFile(ctx context.Context, containerID string, path string) (io.ReadCloser, error) {
	return c.util.CopyFromContainer(ctx, containerID, path)
}

func (c dockerUtilConfigClient) getEnv(ctx context.Context, containerID string) ([]string, error) {
	container, err := c.util.Inspect(ctx, containerID, false)
	if err != nil {
		return nil, err
	}
	if container.Config == nil {
		return nil, nil
	}
	return container.Config.Env, nil
}

func (c dockerUtilConfigClient) getCommandline(ctx context.Context, containerID string) (TargetCommandline, error) {
	container, err := c.util.Inspect(ctx, containerID, false)
	if err != nil {
		return TargetCommandline{}, err
	}

	workingDir := ""
	if container.Config != nil {
		workingDir = container.Config.WorkingDir
	}
	return targetCommandlineFromDockerConfig(container.Path, container.Args, workingDir), nil
}

func targetCommandlineFromDockerConfig(commandPath string, commandArgs []string, workingDir string) TargetCommandline {
	args := make([]string, 0, len(commandArgs)+1)
	if commandPath != "" {
		args = append(args, commandPath)
	}
	args = append(args, commandArgs...)
	if workingDir == "" {
		workingDir = "/"
	}

	return TargetCommandline{
		Args:       args,
		WorkingDir: workingDir,
	}
}
