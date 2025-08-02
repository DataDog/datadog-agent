// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package env

import (
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

const (
	defaultLinuxDockerSocket           = "/var/run/docker.sock"
	defaultWindowsDockerSocketPath     = "//./pipe/docker_engine"
	defaultLinuxContainerdSocket       = "/var/run/containerd/containerd.sock"
	defaultWindowsContainerdSocketPath = "//./pipe/containerd-containerd"
	defaultLinuxCrioSocket             = "/var/run/crio/crio.sock"
	defaultHostMountPrefix             = "/host"
	defaultPodmanContainersStoragePath = "/var/lib/containers/storage"
	unixSocketPrefix                   = "unix://"
	winNamedPipePrefix                 = "npipe://"
	defaultNVMLLibraryName             = "libnvidia-ml.so.1"

	socketTimeout = 500 * time.Millisecond
)

func init() {
	registerFeature(Docker)
	registerFeature(Containerd)
	registerFeature(Cri)
	registerFeature(Crio)
	registerFeature(Kubernetes)
	registerFeature(ECSEC2)
	registerFeature(ECSFargate)
	registerFeature(EKSFargate)
	registerFeature(KubeOrchestratorExplorer)
	registerFeature(ECSOrchestratorExplorer)
	registerFeature(CloudFoundry)
	registerFeature(Podman)
	registerFeature(PodResources)
	registerFeature(NVML)
}

// IsAnyContainerFeaturePresent checks if any of known container features is present
func IsAnyContainerFeaturePresent() bool {
	return IsFeaturePresent(Docker) ||
		IsFeaturePresent(Containerd) ||
		IsFeaturePresent(Cri) ||
		IsFeaturePresent(Crio) ||
		IsFeaturePresent(Kubernetes) ||
		IsFeaturePresent(ECSEC2) ||
		IsFeaturePresent(ECSFargate) ||
		IsFeaturePresent(EKSFargate) ||
		IsFeaturePresent(CloudFoundry) ||
		IsFeaturePresent(Podman)
}

func detectContainerFeatures(features FeatureMap, cfg model.Reader) {
	detectKubernetes(features, cfg)
	detectDocker(features)
	detectCriRuntimes(features, cfg)
	detectAWSEnvironments(features, cfg)
	detectCloudFoundry(features, cfg)
	detectPodman(features, cfg)
	detectPodResources(features, cfg)
	detectNVML(features, cfg)
}

func detectKubernetes(features FeatureMap, cfg model.Reader) {
	if IsKubernetes() {
		features[Kubernetes] = struct{}{}
		if cfg.GetBool("orchestrator_explorer.enabled") {
			features[KubeOrchestratorExplorer] = struct{}{}
		}
	}
}

func detectDocker(features FeatureMap) {
	if _, dockerHostSet := os.LookupEnv("DOCKER_HOST"); dockerHostSet {
		features[Docker] = struct{}{}
	} else {
		for _, defaultDockerSocketPath := range getDefaultDockerPaths() {
			exists, reachable := socket.IsAvailable(defaultDockerSocketPath, socketTimeout)
			if exists && !reachable {
				log.Infof("Agent found Docker socket at: %s but socket not reachable (permissions?)", defaultDockerSocketPath)
				continue
			}

			if exists && reachable {
				features[Docker] = struct{}{}

				// Even though it does not modify configuration, using the OverrideFunc mechanism for uniformity
				model.AddOverrideFunc(func(model.Config) {
					os.Setenv("DOCKER_HOST", getDefaultSocketPrefix()+defaultDockerSocketPath)
				})
				break
			}
		}
	}
}

// detectCriRuntimes checks for both containerd and crio runtimes
func detectCriRuntimes(features FeatureMap, cfg model.Reader) {
	// CRI Socket - Do not automatically default socket path if the Agent runs in Docker
	// as we'll very likely discover the containerd instance wrapped by Docker.
	criSocket := cfg.GetString("cri_socket_path")

	// If no cri_socket_path is provided and the Agent is not running in Docker, check default paths
	if criSocket == "" && !IsDockerRuntime() {
		for _, defaultCriPath := range getDefaultCriPaths() {
			// Check default CRI paths
			criSocket = checkCriSocket(defaultCriPath)
			if criSocket != "" {
				model.AddOverride("cri_socket_path", criSocket)
				// Currently we do not support multiple CRI paths
				break
			}
		}
	} else {
		// Check manually provided CRI socket path
		criSocket = checkCriSocket(criSocket)
	}

	// If a valid CRI socket path was found, determine the runtime (containerd or crio)
	if criSocket != "" {
		if isCriSupported() {
			features[Cri] = struct{}{}
		}
		if strings.Contains(criSocket, "containerd") {
			features[Containerd] = struct{}{}
			mergeContainerdNamespaces(cfg)
		} else if strings.Contains(criSocket, "crio") {
			features[Crio] = struct{}{}
		}
	}
}

func checkCriSocket(socketPath string) string {
	// Check if the socket exists and is reachable
	exists, reachable := socket.IsAvailable(socketPath, socketTimeout)
	if exists && reachable {
		log.Infof("Agent found cri socket at: %s", socketPath)
		return socketPath
	} else if exists && !reachable {
		log.Infof("Agent found cri socket at: %s but socket not reachable (permissions?)", socketPath)
	}
	return ""
}

func mergeContainerdNamespaces(cfg model.Reader) {
	// Merge containerd_namespace with containerd_namespaces
	namespaces := merge(
		cfg.GetStringSlice("containerd_namespaces"),
		cfg.GetStringSlice("containerd_namespace"),
	)

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

	model.AddOverride("containerd_namespace", convertedNamespaces)
	model.AddOverride("containerd_namespaces", convertedNamespaces)
}

func isCriSupported() bool {
	// Containerd support was historically meant for K8S
	// However, containerd is now used standalone elsewhere.
	return IsKubernetes()
}

func detectAWSEnvironments(features FeatureMap, cfg model.Reader) {
	if IsECSFargate() {
		features[ECSFargate] = struct{}{}
		if cfg.GetBool("orchestrator_explorer.enabled") &&
			cfg.GetBool("ecs_task_collection_enabled") {
			features[ECSOrchestratorExplorer] = struct{}{}
		}
		return
	}

	if cfg.GetBool("eks_fargate") {
		features[EKSFargate] = struct{}{}
		features[Kubernetes] = struct{}{}
		return
	}

	if IsECS() {
		features[ECSEC2] = struct{}{}
		if cfg.GetBool("orchestrator_explorer.enabled") &&
			cfg.GetBool("ecs_task_collection_enabled") {
			features[ECSOrchestratorExplorer] = struct{}{}
		}
	}
}

func detectCloudFoundry(features FeatureMap, cfg model.Reader) {
	if cfg.GetBool("cloud_foundry") {
		features[CloudFoundry] = struct{}{}
	}
}

func detectPodman(features FeatureMap, cfg model.Reader) {
	podmanDbPath := cfg.GetString("podman_db_path")
	if podmanDbPath != "" {
		features[Podman] = struct{}{}
		return
	}
	for _, defaultPath := range getDefaultPodmanPaths() {
		if _, err := os.Stat(defaultPath); err == nil {
			features[Podman] = struct{}{}
			return
		}
	}
}

func detectPodResources(features FeatureMap, cfg model.Reader) {
	// We only check the path from config. Default socket path is defined in the config,
	// without the unix:/// prefix, as socket.IsAvailable receives a filesystem path.
	socketPath := cfg.GetString("kubernetes_kubelet_podresources_socket")

	exists, reachable := socket.IsAvailable(socketPath, socketTimeout)
	if exists && reachable {
		log.Infof("Agent found PodResources socket at %s", socketPath)
		features[PodResources] = struct{}{}
	} else if exists && !reachable {
		log.Infof("Agent found PodResources socket at %s but socket not reachable (permissions?)", socketPath)
	} else {
		log.Infof("Agent did not find PodResources socket at %s", socketPath)
	}
}

func detectNVML(features FeatureMap, _ model.Reader) {
	// Use dlopen to search for the library to avoid importing the go-nvml package here,
	// which is 1MB in size and would increase the agent binary size, when we don't really
	// need it for anything else.
	if err := system.CheckLibraryExists(defaultNVMLLibraryName); err != nil {
		log.Debugf("Agent did not find NVML library: %v", err)
		return
	}

	features[NVML] = struct{}{}
	log.Infof("Agent found NVML library")
}

func getHostMountPrefixes() []string {
	if IsContainerized() {
		return []string{"", defaultHostMountPrefix}
	}
	return []string{""}
}

func getDefaultSocketPrefix() string {
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
		paths = append(
			paths,
			path.Join(prefix, defaultLinuxContainerdSocket),
			path.Join(prefix, defaultLinuxCrioSocket),
		)
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
