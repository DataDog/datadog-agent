// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"

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

// ConfigEnvVarPredicate returns whether an environment variable should be read.
type ConfigEnvVarPredicate func(name string) bool

// CollectedConfig is the config data collected for one integration target.
type CollectedConfig struct {
	Integration string
	Runtime     RuntimeType
	RuntimeID   string
	ConfigFiles []ConfigFile
	EnvVars     []ConfigEnvVar
}

// TargetCommandline is a candidate process command line associated with the target.
type TargetCommandline struct {
	Args       []string
	WorkingDir string
}

// UnverifiedConfigFilePath is a config file path that has not crossed the
// runtime reader's path-validation boundary.
type UnverifiedConfigFilePath string

// VerifiedConfigFilePath is a cleaned absolute config file path without parent
// traversal or control characters. Its fields are private so values can only
// be created through VerifyConfigFilePath.
type VerifiedConfigFilePath struct {
	value string
}

// VerifyConfigFilePath validates and cleans path, returning a value safe to
// pass to a runtime reader.
func VerifyConfigFilePath(unverified UnverifiedConfigFilePath) (VerifiedConfigFilePath, error) {
	value, err := verifyConfigFileLocation(string(unverified))
	if err != nil {
		return VerifiedConfigFilePath{}, err
	}
	return VerifiedConfigFilePath{value: value}, nil
}

// String returns the cleaned absolute path.
func (p VerifiedConfigFilePath) String() string {
	return p.value
}

// UnverifiedConfigFilePattern is a config file search pattern that has not
// crossed the runtime reader's path-validation boundary.
type UnverifiedConfigFilePattern string

// VerifiedConfigFilePattern is a cleaned absolute config file search pattern
// without parent traversal or control characters. Its fields are private so
// values can only be created through VerifyConfigFilePattern.
type VerifiedConfigFilePattern struct {
	value string
}

// VerifyConfigFilePattern validates and cleans pattern, returning a value safe
// to pass to a runtime reader.
func VerifyConfigFilePattern(unverified UnverifiedConfigFilePattern) (VerifiedConfigFilePattern, error) {
	value, err := verifyConfigFileLocation(string(unverified))
	if err != nil {
		return VerifiedConfigFilePattern{}, err
	}
	return VerifiedConfigFilePattern{value: value}, nil
}

// String returns the cleaned absolute pattern.
func (p VerifiedConfigFilePattern) String() string {
	return p.value
}

// verifyConfigFileLocation returns a cleaned absolute path or pattern after
// rejecting inputs unsafe to pass to a container runtime.
func verifyConfigFileLocation(value string) (string, error) {
	if value == "" {
		return "", errors.New("empty config file path")
	}
	if !path.IsAbs(value) {
		return "", fmt.Errorf("config file path %q is not absolute", value)
	}
	for _, element := range strings.Split(value, "/") {
		if element == ".." {
			return "", fmt.Errorf("config file path %q contains parent traversal", value)
		}
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("config file path %q contains a control character", value)
		}
	}
	return path.Clean(value), nil
}

// ConfigFilePathMatcher returns whether a verified config file path should be
// included in file-discovery results.
type ConfigFilePathMatcher func(VerifiedConfigFilePath) (bool, error)

// ConfigReader is the runtime-specific config access layer managed by the scheduler.
type ConfigReader interface {
	Runtime() RuntimeType
	ReadFile(context.Context, VerifiedConfigFilePath) (ConfigFile, error)
	// FindFiles uses searchPattern as a conservative runtime-compatible candidate
	// filter, then returns regular-file paths accepted by matches in lexical
	// order. It returns at most maxMatches paths and reports whether additional
	// matches were omitted.
	FindFiles(ctx context.Context, searchPattern VerifiedConfigFilePattern, maxMatches int, matches ConfigFilePathMatcher) (paths []VerifiedConfigFilePath, limited bool, err error)
	ReadEnvVars(context.Context, ConfigEnvVarPredicate) (map[string]string, error)
	ReadRuntimeCommandline(context.Context) (TargetCommandline, error)
	ReadLiveProcessCommandlines(context.Context) []TargetCommandline
	Close()
}

type configReaderFactory func(target) (ConfigReader, error)

// ConfigCollector reads integration-specific config data through a collector reader.
type ConfigCollector interface {
	// CanCollectFromProcess returns whether the collector can use the process command line for collection.
	CanCollectFromProcess(TargetCommandline) bool
	Collect(context.Context, ConfigReader) (CollectedConfig, error)
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
		return resolvedTarget, true
	case "kubernetes_pod":
		return target{}, false
	}

	if runtime != "container" && runtime != "containerd" {
		return target{}, false
	}

	// Concrete container IDs need workloadmeta to distinguish Kubernetes-owned
	// containerd containers from standalone Docker containers and unsupported
	// runtimes. Pod-level IDs are intentionally skipped until there is a clear
	// single-container selection rule.
	if r.store == nil {
		return target{}, false
	}

	// AD schedules container services for Kubernetes pods as container://<id>;
	// use the Kubernetes reader only for containerd-backed pod containers.
	pod, err := r.store.GetKubernetesPodForContainer(id)
	if err != nil || pod == nil {
		if runtime != "container" {
			return target{}, false
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

	if runtime == "container" {
		container, err := r.store.GetContainer(id)
		if err != nil || container == nil || container.Runtime != workloadmeta.ContainerRuntimeContainerd {
			return target{}, false
		}
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
