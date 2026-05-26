// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"strings"

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

// ConfigReader is the runtime-specific config access layer used by config collectors.
type ConfigReader interface {
	Runtime() RuntimeType
}

type configReaderFactory func(target) (ConfigReader, error)

type configCollector interface {
	Run(context.Context, ConfigReader) error
}

var configReaders = map[RuntimeType]configReaderFactory{}

var configCollectors = map[string]configCollector{}

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

	switch runtime {
	case "process":
		resolvedTarget.runtime = RuntimeHost
		return resolvedTarget, true
	case "docker":
		resolvedTarget.runtime = RuntimeDocker
	}

	if r.store == nil {
		return resolvedTarget, resolvedTarget.runtime == RuntimeDocker
	}

	pod, err := r.store.GetKubernetesPodForContainer(id)
	if err != nil || pod == nil {
		return resolvedTarget, resolvedTarget.runtime == RuntimeDocker
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
