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
	"fmt"
	"strconv"
	"strings"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	containerdutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	criutil "github.com/DataDog/datadog-agent/pkg/util/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	containerdoci "github.com/containerd/containerd/v2/pkg/oci"
)

const (
	kubernetesContainerdNamespace = "k8s.io"
	kubernetesReadFileTimeout     = 5 * time.Second
	kubernetesReadFileOutputLimit = maxConfigFileSize + 1
)

// kubernetesConfigClient is the narrow runtime boundary the Kubernetes reader
// needs: CRI for file bytes and containerd for the OCI process spec.
type kubernetesConfigClient interface {
	execSync(context.Context, string, []string, time.Duration) ([]byte, []byte, int32, error)
	containerSpec(context.Context, string) (*containerdoci.Spec, error)
	close()
}

var newKubernetesConfigClient = func() (kubernetesConfigClient, error) {
	cri, err := criutil.GetUtil()
	if err != nil {
		return nil, err
	}

	containerd, err := containerdutil.NewContainerdUtil()
	if err != nil {
		return nil, err
	}

	return kubernetesRuntimeConfigClient{
		cri:        cri,
		containerd: containerd,
		namespace:  kubernetesContainerdNamespace,
	}, nil
}

type kubernetesConfigReader struct {
	containerID string
	client      kubernetesConfigClient
	store       workloadmeta.Component
}

func newKubernetesConfigReader(t target, store workloadmeta.Component) (ConfigReader, error) {
	if t.runtime != RuntimeKubernetes {
		return nil, fmt.Errorf("unsupported runtime %q", t.runtime)
	}
	if t.entityID == "" {
		return nil, errors.New("empty kubernetes container id")
	}

	client, err := newKubernetesConfigClient()
	if err != nil {
		return nil, err
	}

	return &kubernetesConfigReader{
		containerID: t.entityID,
		client:      client,
		store:       store,
	}, nil
}

func newKubernetesConfigReaderWithClient(containerID string, client kubernetesConfigClient) *kubernetesConfigReader {
	return &kubernetesConfigReader{
		containerID: containerID,
		client:      client,
	}
}

func (r *kubernetesConfigReader) Runtime() RuntimeType {
	return RuntimeKubernetes
}

func (r *kubernetesConfigReader) Close() {
	r.client.close()
}

func (r *kubernetesConfigReader) ReadFile(ctx context.Context, filePath string) (ConfigFile, error) {
	cleanPath, err := cleanContainerFilePath(filePath)
	if err != nil {
		return ConfigFile{}, err
	}

	stdout, stderr, exitCode, err := r.client.execSync(ctx, r.containerID, kubernetesReadFileCommand(cleanPath), kubernetesReadFileTimeout)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("exec read config file in kubernetes container: %w", err)
	}
	if exitCode != 0 {
		return ConfigFile{}, kubernetesExecExitError(exitCode, stderr)
	}

	content, truncated, err := readLimitedFileContent(bytes.NewReader(stdout), maxConfigFileSize)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("read kubernetes config file output: %w", err)
	}

	return ConfigFile{
		Path:      cleanPath,
		Content:   content,
		Truncated: truncated,
	}, nil
}

func kubernetesReadFileCommand(cleanPath string) []string {
	return []string{"head", "-c", strconv.Itoa(kubernetesReadFileOutputLimit), cleanPath}
}

func (r *kubernetesConfigReader) ReadEnvVars(ctx context.Context, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}

	spec, err := r.client.containerSpec(ctx, r.containerID)
	if err != nil {
		return nil, fmt.Errorf("get kubernetes container OCI spec: %w", err)
	}
	if spec == nil || spec.Process == nil {
		return map[string]string{}, nil
	}

	return filterEnvVars(spec.Process.Env, names), nil
}

func (r *kubernetesConfigReader) ReadRuntimeCommandline(ctx context.Context) (TargetCommandline, error) {
	spec, err := r.client.containerSpec(ctx, r.containerID)
	if err != nil {
		return TargetCommandline{}, fmt.Errorf("get kubernetes container OCI spec: %w", err)
	}

	commandline := TargetCommandline{WorkingDir: "/"}
	if spec == nil || spec.Process == nil {
		return commandline, nil
	}

	commandline.Args = append([]string(nil), spec.Process.Args...)
	if spec.Process.Cwd != "" {
		commandline.WorkingDir = spec.Process.Cwd
	}

	return commandline, nil
}

func (r *kubernetesConfigReader) ReadLiveProcessCommandlines(ctx context.Context) []TargetCommandline {
	return readContainerProcessCommandlines(ctx, r.store, r.containerID, readLiveProcessWorkingDir)
}

func kubernetesExecExitError(exitCode int32, stderr []byte) error {
	stderrText := strings.TrimSpace(string(stderr))
	if stderrText == "" {
		return fmt.Errorf("exec read config file in kubernetes container exited with code %d", exitCode)
	}
	return fmt.Errorf("exec read config file in kubernetes container exited with code %d: %s", exitCode, stderrText)
}

type kubernetesRuntimeConfigClient struct {
	cri        criExecClient
	containerd containerdutil.ContainerdItf
	namespace  string
}

type criExecClient interface {
	ExecSync(context.Context, string, []string, time.Duration) ([]byte, []byte, int32, error)
}

func (c kubernetesRuntimeConfigClient) execSync(ctx context.Context, containerID string, cmd []string, timeout time.Duration) ([]byte, []byte, int32, error) {
	return c.cri.ExecSync(ctx, containerID, cmd, timeout)
}

func (c kubernetesRuntimeConfigClient) close() {
	if err := c.containerd.Close(); err != nil {
		log.Debugf("failed to close containerd client for config files discovery: %v", err)
	}
}

func (c kubernetesRuntimeConfigClient) containerSpec(ctx context.Context, containerID string) (*containerdoci.Spec, error) {
	container, err := c.containerd.ContainerWithContext(ctx, c.namespace, containerID)
	if err != nil {
		return nil, err
	}

	info, err := c.containerd.Info(c.namespace, container)
	if err != nil {
		return nil, err
	}

	return c.containerd.Spec(c.namespace, info, containerdutil.DefaultAllowedSpecMaxSize)
}
