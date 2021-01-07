// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Feature represents a feature of current environment
type Feature int

const (
	// Docker socket present
	Docker Feature = iota
	// Containerd socket present
	Containerd
	// Cri is any cri socket present
	Cri
	// Kubernetes environment
	Kubernetes
	// ECSFargate environment
	ECSFargate
	// EKSFargate environment
	EKSFargate
)

const (
	defaultLinuxDockerSocket       = "/var/run/docker.sock"
	defaultWindowsDockerSocketPath = "//./pipe/docker_engine"
	defaultLinuxContainerdSocket   = "/var/run/containerd/containerd.sock"
	defaultLinuxCrioSocket         = "/var/run/crio/crio.sock"
	defaultHostMountPrefix         = "/host"
)

// FeatureMap represents all detected features
type FeatureMap map[Feature]struct{}

var (
	detectedFeatures = make(FeatureMap)
)

// GetDetectedFeatures returns all detected features (detection only performed once)
func GetDetectedFeatures() FeatureMap {
	return detectedFeatures
}

// IsFeaturePresent returns if a particular feature is activated
func IsFeaturePresent(feature Feature) bool {
	_, found := detectedFeatures[feature]
	return found
}

func detectFeatures() {
	if Datadog.GetBool("autoconf_from_environment") {
		detectContainerFeatures()
		log.Debugf("Features detected from environment: %v", detectedFeatures)
	}
}

func detectContainerFeatures() {
	// Docker
	if _, dockerHostSet := os.LookupEnv("DOCKER_HOST"); dockerHostSet {
		detectedFeatures[Docker] = struct{}{}
	} else {
		defaultDockerSocketPath := getDefaultDockerPath()
		_, err := os.Stat(defaultDockerSocketPath)
		if err == nil {
			detectedFeatures[Docker] = struct{}{}
			// Even though it does not modify configuration, using the OverrideFunc mechanism for uniformity
			AddOverrideFunc(func(Config) {
				os.Setenv("DOCKER_HOST", "unix://"+defaultDockerSocketPath)
			})
		}
	}

	// CRI Socket - Do not automatically default socket path if Docker is running as Docker is now wrapping containerd
	criSocket := Datadog.GetString("cri_socket_path")
	if len(criSocket) == 0 {
		for _, defaultCriPath := range getDefaultCriPaths() {
			if _, err := os.Stat(defaultCriPath); err == nil {
				criSocket = defaultCriPath
				AddOverride("cri_socket_path", defaultCriPath)
				// Currently we do not support multiple CRI paths
				break
			}
		}
	}

	if criSocket != "" {
		detectedFeatures[Cri] = struct{}{}
		if strings.Contains(criSocket, "containerd") {
			detectedFeatures[Containerd] = struct{}{}
		}
	}

	if IsKubernetes() {
		detectedFeatures[Kubernetes] = struct{}{}
	}

	if IsECSFargate() {
		detectedFeatures[ECSFargate] = struct{}{}
	}

	if Datadog.GetBool("eks_fargate") {
		detectedFeatures[EKSFargate] = struct{}{}
		detectedFeatures[Kubernetes] = struct{}{}
	}
}

func getHostMountPrefix() string {
	if IsContainerized() {
		return defaultHostMountPrefix
	}
	return ""
}

func getDefaultDockerPath() string {
	if runtime.GOOS == "windows" {
		return defaultWindowsDockerSocketPath
	}
	return path.Join(getHostMountPrefix(), defaultLinuxDockerSocket)
}

func getDefaultCriPaths() []string {
	return []string{path.Join(getHostMountPrefix(), defaultLinuxContainerdSocket), path.Join(getHostMountPrefix(), defaultLinuxCrioSocket)}
}
