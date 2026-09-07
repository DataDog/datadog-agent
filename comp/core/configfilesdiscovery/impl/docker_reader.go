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
	"slices"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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
	store       workloadmeta.Component
}

func newDockerConfigReader(t target, store workloadmeta.Component) (ConfigReader, error) {
	if t.runtime != RuntimeDocker {
		return nil, fmt.Errorf("unsupported runtime %q", t.runtime)
	}
	if t.entityID == "" {
		return nil, errors.New("empty docker container id")
	}

	client, err := newDockerConfigClient()
	if err != nil {
		return nil, err
	}

	return &dockerConfigReader{
		containerID: t.entityID,
		client:      client,
		store:       store,
	}, nil
}

func (r *dockerConfigReader) Runtime() RuntimeType {
	return RuntimeDocker
}

func (r *dockerConfigReader) Close() {}

func (r *dockerConfigReader) ReadFile(ctx context.Context, filePath VerifiedConfigFilePath) (ConfigFile, error) {
	body, err := r.client.getFile(ctx, r.containerID, filePath.String())
	if err != nil {
		return ConfigFile{}, fmt.Errorf("copy config file from docker container: %w", err)
	}
	defer body.Close()

	return readConfigFileFromDockerArchive(body, filePath.String())
}

// ReadMatchingFiles reads matching regular files from an archive rooted where
// Docker can observe, rather than traverse, symlinks below the trusted root.
func (r *dockerConfigReader) ReadMatchingFiles(ctx context.Context, search ConfigFileSearch, maxMatches int, matches ConfigFilePathMatcher) ([]ConfigFileReadResult, bool, error) {
	if maxMatches <= 0 {
		return nil, false, errors.New("maximum file matches must be positive")
	}

	searchRoot := configFileSearchRoot(search)
	body, err := r.client.getFile(ctx, r.containerID, searchRoot.String())
	if err != nil {
		return nil, false, fmt.Errorf("copy config file pattern root from docker container: %w", err)
	}
	defer body.Close()

	results, limited, err := readMatchingRegularFilesFromDockerArchive(body, searchRoot, search, maxMatches, matches)
	if err != nil {
		return nil, false, err
	}
	return results, limited, nil
}

func (r *dockerConfigReader) ReadEnvVars(ctx context.Context, predicate ConfigEnvVarPredicate) (map[string]string, error) {
	if predicate == nil {
		return map[string]string{}, nil
	}

	envEntries, err := r.client.getEnv(ctx, r.containerID)
	if err != nil {
		return nil, fmt.Errorf("get docker container env: %w", err)
	}

	return filterEnvVars(envEntries, predicate), nil
}

func (r *dockerConfigReader) ReadRuntimeCommandline(ctx context.Context) (TargetCommandline, error) {
	commandline, err := r.client.getCommandline(ctx, r.containerID)
	if err != nil {
		return TargetCommandline{}, fmt.Errorf("get docker container command line: %w", err)
	}
	return commandline, nil
}

func (r *dockerConfigReader) ReadLiveProcessCommandlines(context.Context) []TargetCommandline {
	return readContainerProcessCommandlines(r.store, r.containerID)
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

// readMatchingRegularFilesFromDockerArchive returns bounded matching regular
// files in lexical order without following symlink archive entries.
func readMatchingRegularFilesFromDockerArchive(r io.Reader, searchRoot VerifiedConfigFilePath, search ConfigFileSearch, maxMatches int, matches ConfigFilePathMatcher) ([]ConfigFileReadResult, bool, error) {
	tr := tar.NewReader(r)
	var results []ConfigFileReadResult
	seen := make(map[VerifiedConfigFilePath]struct{})
	limited := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, fmt.Errorf("read docker archive: %w", err)
		}
		if !isRegularTarEntry(header) {
			continue
		}

		entryPath, err := verifyDockerArchiveEntryPath(searchRoot, header.Name)
		if err != nil {
			continue
		}
		if !search.Contains(entryPath) {
			continue
		}
		matched, err := matches(entryPath)
		if err != nil {
			return nil, false, fmt.Errorf("match docker archive entry %q: %w", header.Name, err)
		}
		if !matched {
			continue
		}
		if _, found := seen[entryPath]; found {
			continue
		}
		seen[entryPath] = struct{}{}

		if len(results) == maxMatches {
			limited = true
			if entryPath.String() > results[len(results)-1].Path().String() {
				continue
			}
		}

		content, truncated, err := readLimitedFileContent(tr, maxConfigFileSize)
		if err != nil {
			return nil, false, fmt.Errorf("read docker archive entry %q: %w", header.Name, err)
		}
		file := ConfigFile{
			Path:      entryPath.String(),
			Content:   content,
			Truncated: truncated,
		}
		results = append(results, NewConfigFileReadResult(entryPath, file))
		slices.SortFunc(results, func(left, right ConfigFileReadResult) int {
			return strings.Compare(left.Path().String(), right.Path().String())
		})
		if len(results) > maxMatches {
			results = results[:maxMatches]
			limited = true
		}
	}
	return results, limited, nil
}

// verifyDockerArchiveEntryPath verifies and returns the absolute container path
// for an archive entry copied from searchRoot.
func verifyDockerArchiveEntryPath(searchRoot VerifiedConfigFilePath, entryName string) (VerifiedConfigFilePath, error) {
	if path.IsAbs(entryName) {
		return VerifyConfigFilePath(UnverifiedConfigFilePath(entryName))
	}
	entryPath, err := VerifyConfigFilePath(UnverifiedConfigFilePath("/" + entryName))
	if err != nil {
		return VerifiedConfigFilePath{}, err
	}
	cleanEntry := strings.TrimPrefix(entryPath.String(), "/")
	cleanRoot := strings.TrimPrefix(searchRoot.String(), "/")
	if cleanEntry == cleanRoot || strings.HasPrefix(cleanEntry, cleanRoot+"/") {
		return VerifiedConfigFilePath{value: "/" + cleanEntry}, nil
	}
	rootBase := path.Base(searchRoot.String())
	if cleanEntry == rootBase || strings.HasPrefix(cleanEntry, rootBase+"/") {
		return VerifiedConfigFilePath{
			value: path.Clean(path.Join(path.Dir(searchRoot.String()), cleanEntry)),
		}, nil
	}
	return VerifiedConfigFilePath{
		value: path.Clean(path.Join(searchRoot.String(), cleanEntry)),
	}, nil
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
