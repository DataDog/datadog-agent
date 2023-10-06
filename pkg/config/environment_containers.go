// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package config

import (
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

const (
	defaultLinuxDockerSocket           = "/var/run/docker.sock"
	defaultWindowsDockerSocketPath     = "//./pipe/docker_engine"
	defaultLinuxContainerdSocket       = "/var/run/containerd/containerd.sock"
	defaultWindowsContainerdSocketPath = "//./pipe/containerd-containerd"
	defaultLinuxCrioSocket             = "/var/run/crio/crio.sock"
	defaultHostMountPrefix             = "/host"
	defaultPodmanContainersStoragePath = "/var/lib/containers"
	unixSocketPrefix                   = "unix://"
	winNamedPipePrefix                 = "npipe://"

	socketTimeout = 500 * time.Millisecond
)

func init() {
	registerFeature(Docker)
	registerFeature(Containerd)
	registerFeature(Cri)
	registerFeature(Kubernetes)
	registerFeature(ECSEC2)
	registerFeature(ECSFargate)
	registerFeature(EKSFargate)
	registerFeature(KubeOrchestratorExplorer)
	registerFeature(CloudFoundry)
	registerFeature(Podman)
}

// IsAnyContainerFeaturePresent checks if any of known container features is present
func IsAnyContainerFeaturePresent() bool {
	return IsFeaturePresent(Docker) ||
		IsFeaturePresent(Containerd) ||
		IsFeaturePresent(Cri) ||
		IsFeaturePresent(Kubernetes) ||
		IsFeaturePresent(ECSEC2) ||
		IsFeaturePresent(ECSFargate) ||
		IsFeaturePresent(EKSFargate) ||
		IsFeaturePresent(CloudFoundry) ||
		IsFeaturePresent(Podman)
}

func detectContainerFeatures(features FeatureMap) {
	detectKubernetes(features)
	detectDocker(features)
	detectContainerd(features)
	detectAWSEnvironments(features)
	detectCloudFoundry(features)
	detectPodman(features)
}

func detectKubernetes(features FeatureMap) {
	if IsKubernetes() {
		features[Kubernetes] = struct{}{}
		if Datadog.GetBool("orchestrator_explorer.enabled") {
			features[KubeOrchestratorExplorer] = struct{}{}
		}
	}
}

func detectDocker(features FeatureMap) {
	if _, dockerHostSet := os.LookupEnv("DOCKER_HOST"); dockerHostSet {
		features[Docker] = struct{}{}
	} else {
		for _, defaultDockerSocketPath := range getDefaultDockerPaths() {
			exists, reachable := system.CheckSocketAvailable(defaultDockerSocketPath, socketTimeout)
			if exists && !reachable {
				log.Infof("Agent found Docker socket at: %s but socket not reachable (permissions?)", defaultDockerSocketPath)
				continue
			}

			if exists && reachable {
				features[Docker] = struct{}{}

				// Even though it does not modify configuration, using the OverrideFunc mechanism for uniformity
				AddOverrideFunc(func(Config) {
					os.Setenv("DOCKER_HOST", getDefaultDockerSocketType()+defaultDockerSocketPath)
				})
				break
			}
		}
	}
}

func detectContainerd(features FeatureMap) {
	// CRI Socket - Do not automatically default socket path if the Agent runs in Docker
	// as we'll very likely discover the containerd instance wrapped by Docker.
	criSocket := Datadog.GetString("cri_socket_path")
	if criSocket == "" && !IsDockerRuntime() {
		for _, defaultCriPath := range getDefaultCriPaths() {
			exists, reachable := system.CheckSocketAvailable(defaultCriPath, socketTimeout)
			if exists && !reachable {
				log.Infof("Agent found cri socket at: %s but socket not reachable (permissions?)", defaultCriPath)
				continue
			}

			if exists && reachable {
				criSocket = defaultCriPath
				AddOverride("cri_socket_path", defaultCriPath)
				// Currently we do not support multiple CRI paths
				break
			}
		}
	}

	if criSocket != "" {
		if isCriSupported() {
			features[Cri] = struct{}{}
		}

		if strings.Contains(criSocket, "containerd") {
			features[Containerd] = struct{}{}
		}
	}

	// Merge containerd_namespace with containerd_namespaces
	namespaces := merge(Datadog.GetStringSlice("containerd_namespaces"), Datadog.GetStringSlice("containerd_namespace"))

	// Workaround: convert to []interface{}.
	// The MergeConfigOverride func in "github.com/DataDog/viper" (tested in
	// v1.10.0) raises an error if we send a []string{} in AddOverride():
	// "svType != tvType; key=containerd_namespace, st=[]interface {}, tt=[]string, sv=[], tv=[]"
	// The reason is that when reading from a config file, all the arrays are
	// considered as []interface{} by Viper, and the merge fails when the types
	// are different.
	convertedNamespaces := make([]interface{}, len(namespaces))
	for i, namespace := range namespaces {
		convertedNamespaces[i] = namespace
	}

	AddOverride("containerd_namespace", convertedNamespaces)
	AddOverride("containerd_namespaces", convertedNamespaces)
}

func isCriSupported() bool {
	// Containerd support was historically meant for K8S
	// However, containerd is now used standalone elsewhere.
	return IsKubernetes()
}

func detectAWSEnvironments(features FeatureMap) {
	if IsECSFargate() {
		features[ECSFargate] = struct{}{}
		return
	}

	if Datadog.GetBool("eks_fargate") {
		features[EKSFargate] = struct{}{}
		features[Kubernetes] = struct{}{}
		return
	}

	if IsECS() {
		features[ECSEC2] = struct{}{}
	}
}

func detectCloudFoundry(features FeatureMap) {
	if Datadog.GetBool("cloud_foundry") {
		features[CloudFoundry] = struct{}{}
	}
}

func detectPodman(features FeatureMap) {
	for _, defaultPath := range getDefaultPodmanPaths() {
		if _, err := os.Stat(defaultPath); err == nil {
			features[Podman] = struct{}{}
			return
		}
	}
}

func getHostMountPrefixes() []string {
	if IsContainerized() {
		return []string{"", defaultHostMountPrefix}
	}
	return []string{""}
}

func getDefaultDockerSocketType() string {
	if runtime.GOOS == "windows" {
		return winNamedPipePrefix
	}

	return unixSocketPrefix
}

func getDefaultDockerPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{defaultWindowsDockerSocketPath}
	}

	paths := []string{}
	for _, prefix := range getHostMountPrefixes() {
		paths = append(paths, path.Join(prefix, defaultLinuxDockerSocket))
	}
	return paths
}

func getDefaultCriPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{defaultWindowsContainerdSocketPath}
	}

	paths := []string{}
	for _, prefix := range getHostMountPrefixes() {
		paths = append(paths, path.Join(prefix, defaultLinuxContainerdSocket), path.Join(prefix, defaultLinuxCrioSocket))
	}
	return paths
}

func getDefaultPodmanPaths() []string {
	paths := []string{}
	for _, prefix := range getHostMountPrefixes() {
		paths = append(paths, path.Join(prefix, defaultPodmanContainersStoragePath))
	}
	return paths
}

// merge merges and dedupes 2 slices without changing order
func merge(s1, s2 []string) []string {
	dedupe := map[string]struct{}{}
	merged := []string{}

	for _, elem := range append(s1, s2...) {
		if _, seen := dedupe[elem]; !seen {
			merged = append(merged, elem)
		}

		dedupe[elem] = struct{}{}
	}

	return merged
}
