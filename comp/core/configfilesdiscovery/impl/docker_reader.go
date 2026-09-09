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
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	dockerutil "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/moby/moby/api/pkg/stdcopy"
	dockerclient "github.com/moby/moby/client"
)

const (
	dockerExecTimeout     = 5 * time.Second
	dockerFindOutputLimit = 256 * 1024
	dockerExecStderrLimit = 8 * 1024
)

var errDockerExecOutputLimit = errors.New("docker exec output limit reached")

// dockerConfigClient is a narrow Docker interface; reader tests mock it so
// bounded exec, tar decoding, env filtering, and command-line extraction are
// tested without a Docker daemon.
type dockerConfigClient interface {
	getFile(context.Context, string, string) (io.ReadCloser, error)
	execSync(context.Context, string, []string, int) (dockerExecOutput, error)
	getEnv(context.Context, string) ([]string, error)
	getCommandline(context.Context, string) (TargetCommandline, error)
}

// dockerExecOutput contains the bounded output and status of a Docker exec.
type dockerExecOutput struct {
	stdout        []byte
	stderr        []byte
	exitCode      int
	stdoutLimited bool
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

// ReadMatchingFiles discovers names without copying unrelated file contents,
// then reads matching regular files within the trusted root.
func (r *dockerConfigReader) ReadMatchingFiles(ctx context.Context, search ConfigFileSearch, maxMatches int, matches ConfigFilePathMatcher) ([]ConfigFileReadResult, bool, error) {
	if maxMatches <= 0 {
		return nil, false, errors.New("maximum file matches must be positive")
	}

	searchRoot := configFileSearchRoot(search)
	command := []string{"find", "-P", searchRoot.String(), "-type", "f", "-path", search.Pattern().String(), "-print0"}
	output, err := r.client.execSync(ctx, r.containerID, command, dockerFindOutputLimit)
	if err != nil {
		return nil, false, fmt.Errorf("exec find config files in docker container: %w", err)
	}
	if !output.stdoutLimited && output.exitCode != 0 {
		return nil, false, dockerExecExitError(output.exitCode, output.stderr)
	}

	discoveryLimited := output.stdoutLimited
	stdout := output.stdout
	if len(stdout) != 0 && stdout[len(stdout)-1] != 0 {
		discoveryLimited = true
		lastSeparator := bytes.LastIndexByte(stdout, 0)
		if lastSeparator < 0 {
			stdout = nil
		} else {
			stdout = stdout[:lastSeparator+1]
		}
	}

	var paths []VerifiedConfigFilePath
	for _, outputPath := range bytes.Split(stdout, []byte{0}) {
		if len(outputPath) == 0 {
			continue
		}
		filePath, err := VerifyConfigFilePath(UnverifiedConfigFilePath(outputPath))
		if err != nil || !search.Contains(filePath) {
			continue
		}
		matched, err := matches(filePath)
		if err != nil {
			return nil, false, fmt.Errorf("match docker config file %q: %w", filePath.String(), err)
		}
		if matched {
			paths = append(paths, filePath)
		}
	}
	paths, pathsLimited, err := sortAndLimitFilePaths(paths, maxMatches)
	if err != nil {
		return nil, false, err
	}

	var results []ConfigFileReadResult
	for _, filePath := range paths {
		file, err := r.readFileWithinSearch(ctx, searchRoot, filePath)
		if err != nil {
			results = append(results, NewConfigFileReadError(filePath, err))
			continue
		}
		results = append(results, NewConfigFileReadResult(filePath, file))
	}
	return results, discoveryLimited || pathsLimited, nil
}

// readFileWithinSearch revalidates and reads filePath without following a
// symlink observed below searchRoot.
func (r *dockerConfigReader) readFileWithinSearch(ctx context.Context, searchRoot VerifiedConfigFilePath, filePath VerifiedConfigFilePath) (ConfigFile, error) {
	command := dockerReadFileWithinSearchCommand(searchRoot, filePath)
	stdoutLimit := len(filePath.String()) + 1 + maxConfigFileSize + 1
	output, err := r.client.execSync(ctx, r.containerID, command, stdoutLimit)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("exec read scoped config file in docker container: %w", err)
	}
	if output.stdoutLimited {
		return ConfigFile{}, fmt.Errorf("read scoped docker config file %q exceeded its output limit", filePath.String())
	}
	if output.exitCode != 0 {
		return ConfigFile{}, dockerExecExitError(output.exitCode, output.stderr)
	}
	if len(output.stderr) != 0 {
		return ConfigFile{}, fmt.Errorf("read scoped config file in docker container: %s", strings.TrimSpace(string(output.stderr)))
	}

	pathPrefix := append([]byte(filePath.String()), 0)
	if !bytes.HasPrefix(output.stdout, pathPrefix) {
		return ConfigFile{}, fmt.Errorf("config file %q is not a regular file within search root %q", filePath.String(), searchRoot.String())
	}
	content, truncated, err := readLimitedFileContent(bytes.NewReader(output.stdout[len(pathPrefix):]), maxConfigFileSize)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("read docker config file output: %w", err)
	}
	return ConfigFile{Path: filePath.String(), Content: content, Truncated: truncated}, nil
}

// dockerReadFileWithinSearchCommand returns a command that emits the path
// followed by bounded contents only when find observes a regular file.
func dockerReadFileWithinSearchCommand(searchRoot VerifiedConfigFilePath, filePath VerifiedConfigFilePath) []string {
	return []string{
		"find", "-P", searchRoot.String(),
		"-type", "f",
		"-path", escapeFindPathPattern(filePath),
		"-print0",
		"-exec", "head", "-c", strconv.Itoa(maxConfigFileSize + 1), "{}", ";",
	}
}

// dockerExecExitError returns an error containing the exit code and bounded
// stderr from a Docker exec.
func dockerExecExitError(exitCode int, stderr []byte) error {
	stderrText := strings.TrimSpace(string(stderr))
	if stderrText == "" {
		return fmt.Errorf("exec in docker container exited with code %d", exitCode)
	}
	return fmt.Errorf("exec in docker container exited with code %d: %s", exitCode, stderrText)
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

type dockerUtilConfigClient struct {
	util *dockerutil.DockerUtil
}

func (c dockerUtilConfigClient) getFile(ctx context.Context, containerID string, path string) (io.ReadCloser, error) {
	return c.util.CopyFromContainer(ctx, containerID, path)
}

// execSync executes command without a shell and captures bounded stdout and
// stderr from the Docker multiplexed stream.
func (c dockerUtilConfigClient) execSync(ctx context.Context, containerID string, command []string, stdoutLimit int) (dockerExecOutput, error) {
	if stdoutLimit <= 0 {
		return dockerExecOutput{}, errors.New("docker exec stdout limit must be positive")
	}

	execCtx, cancel := context.WithTimeout(ctx, dockerExecTimeout)
	defer cancel()
	client := c.util.RawClient()
	created, err := client.ExecCreate(execCtx, containerID, dockerclient.ExecCreateOptions{
		Cmd:          command,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return dockerExecOutput{}, fmt.Errorf("create docker exec: %w", err)
	}
	attached, err := client.ExecAttach(execCtx, created.ID, dockerclient.ExecAttachOptions{})
	if err != nil {
		return dockerExecOutput{}, fmt.Errorf("attach docker exec: %w", err)
	}
	defer attached.Close()
	// A hijacked connection outlives its HTTP request, so cancellation must
	// explicitly close the connection to unblock StdCopy.
	stopCloseOnCancellation := context.AfterFunc(execCtx, attached.Close)
	defer stopCloseOnCancellation()

	stdout := &dockerExecOutputBuffer{limit: stdoutLimit}
	stderr := &dockerExecOutputBuffer{limit: dockerExecStderrLimit}
	_, copyErr := stdcopy.StdCopy(stdout, stderr, attached.Reader)
	output := dockerExecOutput{
		stdout:        bytes.Clone(stdout.Bytes()),
		stderr:        bytes.Clone(stderr.Bytes()),
		stdoutLimited: stdout.limited,
	}
	if stderr.limited {
		return dockerExecOutput{}, fmt.Errorf("docker exec stderr exceeded %d bytes", dockerExecStderrLimit)
	}
	if copyErr != nil && !errors.Is(copyErr, errDockerExecOutputLimit) {
		if execCtx.Err() != nil {
			return dockerExecOutput{}, execCtx.Err()
		}
		return dockerExecOutput{}, fmt.Errorf("read docker exec output: %w", copyErr)
	}
	if output.stdoutLimited {
		return output, nil
	}

	inspected, err := client.ExecInspect(execCtx, created.ID, dockerclient.ExecInspectOptions{})
	if err != nil {
		return dockerExecOutput{}, fmt.Errorf("inspect docker exec: %w", err)
	}
	output.exitCode = inspected.ExitCode
	return output, nil
}

// dockerExecOutputBuffer stores at most limit bytes and interrupts stdcopy
// when the Docker exec produces more output.
type dockerExecOutputBuffer struct {
	bytes.Buffer
	limit   int
	limited bool
}

// Write appends output up to the configured limit and reports the limit error
// as soon as additional bytes are observed.
func (b *dockerExecOutputBuffer) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.limited = true
		return 0, errDockerExecOutputLimit
	}
	if len(data) <= remaining {
		return b.Buffer.Write(data)
	}

	written, err := b.Buffer.Write(data[:remaining])
	if err != nil {
		return written, err
	}
	b.limited = true
	return written, errDockerExecOutputLimit
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
