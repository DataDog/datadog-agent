// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"strings"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// RuntimeType identifies where an integration's backing service is running.
type RuntimeType string

const (
	// RuntimeKubernetes identifies a service running in a Kubernetes pod.
	RuntimeKubernetes RuntimeType = "k8s"
	// RuntimeDocker identifies a service running in a standalone Docker container.
	RuntimeDocker RuntimeType = "docker"
	// RuntimeHost identifies a service running directly on the host.
	RuntimeHost RuntimeType = "host"
)

type target struct {
	runtime  RuntimeType
	entityID string
}

// ConfigFile is the content read from a runtime-specific config file path.
type ConfigFile struct {
	Path          string
	Content       []byte
	Truncated     bool
	PayloadFormat agentdiscovery.AgentDiscoveryConfigFilePayloadFormat
}

// ConfigEnvVar is an environment variable relevant to a collected integration.
type ConfigEnvVar struct {
	Name  string
	Value string
}

// TargetCommandline is the command line used to start the target service.
type TargetCommandline struct {
	Args       []string
	WorkingDir string
}

// ConfigReader is the runtime-specific config access layer used by config collectors.
type ConfigReader interface {
	Runtime() RuntimeType
	ReadFile(context.Context, string) (ConfigFile, error)
	ReadEnvVars(context.Context, []string) (map[string]string, error)
	ReadCommandline(context.Context) (TargetCommandline, error)
}

type configReaderFactory func(target) (ConfigReader, error)

type configCollector interface {
	Collect(context.Context, ConfigReader) ([]ConfigFile, error)
}

type targetResolver struct {
	store workloadmeta.Component
}

func (r targetResolver) Resolve(config integration.Config) (target, bool) {
	if config.Name == "" || config.ServiceID == "" || !config.IsCheckConfig() {
		return target{}, false
	}

	runtime, id, ok := parseServiceID(config.ServiceID)
	if !ok {
		return target{}, false
	}

	resolvedTarget := target{
		entityID: id,
	}

	// The ServiceID prefix is an AD entity kind, not necessarily the config
	// reader runtime this component needs.
	switch runtime {
	case "process":
		resolvedTarget.runtime = RuntimeHost
		return resolvedTarget, true
	case "docker":
		resolvedTarget.runtime = RuntimeDocker
	case "kubernetes_pod":
		resolvedTarget.runtime = RuntimeKubernetes
		return resolvedTarget, true
	}

	// container:// IDs need workloadmeta to distinguish Kubernetes-owned
	// containers from standalone Docker containers and unsupported runtimes.
	if r.store == nil {
		return resolvedTarget, resolvedTarget.runtime == RuntimeDocker
	}

	// AD schedules container services for Kubernetes pods as container://<id>;
	// prefer the Kubernetes reader when workloadmeta links the container to a pod.
	pod, err := r.store.GetKubernetesPodForContainer(id)
	if err != nil || pod == nil {
		if runtime != "container" {
			return resolvedTarget, resolvedTarget.runtime == RuntimeDocker
		}

		// Standalone container:// services only map to the Docker reader today.
		// Other container runtimes need their own readers before they can run.
		container, err := r.store.GetContainer(id)
		if err != nil || container == nil || container.Runtime != workloadmeta.ContainerRuntimeDocker {
			return target{}, false
		}

		resolvedTarget.runtime = RuntimeDocker
		return resolvedTarget, true
	}

	resolvedTarget.runtime = RuntimeKubernetes
	return resolvedTarget, true
}

func parseServiceID(serviceID string) (string, string, bool) {
	runtime, id, found := strings.Cut(serviceID, "://")
	if !found || runtime == "" || id == "" {
		return "", "", false
	}
	return runtime, id, true
}
