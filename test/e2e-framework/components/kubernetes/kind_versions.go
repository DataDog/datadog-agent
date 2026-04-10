// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/semver/v3"
)

// KindConfig contains the kind version and the kind node image to use
type KindConfig struct {
	KindVersion            string
	NodeImageVersion       string
	UseNewContainerdConfig bool
}

// KindConfigFlags contains flags to generate a kind cluster configuration
// It can be used to generate different kind cluster configurations based on the flags set
// It can be extended in the future to add more configuration options, mount path, featureflags etc.
// It must match the fields in the kind-cluster.yaml template
type KindConfigFlags struct {
	NewContainerdRegistryConfig bool // whether to use the new containerd registry mirror config format (for containerd >= 2.2, used in kubernetes >= v1.32)
	KubeProxyReplacement        bool // whether to set kubeProxyMode to "none" in the kind config
	DualNodeSetup               bool
}

//go:embed kind_versions.json
var kindVersionsJSON []byte

// kubeToKindVersion maps Kubernetes minor version (e.g. "1.35") to kind config.
// Populated at init time from kind_versions.json, which is updated automatically
// by the update-kubernetes-versions CI workflow.
// Source: https://github.com/kubernetes-sigs/kind/releases
var kubeToKindVersion map[string]KindConfig

func init() {
	type kindVersionEntry struct {
		KindVersion      string `json:"kind_version"`
		NodeImageVersion string `json:"node_image_version"`
	}
	var raw map[string]kindVersionEntry
	if err := json.Unmarshal(kindVersionsJSON, &raw); err != nil {
		panic(fmt.Sprintf("failed to parse kind_versions.json: %v", err))
	}
	kubeToKindVersion = make(map[string]KindConfig, len(raw))
	for k, v := range raw {
		kubeToKindVersion[k] = KindConfig{
			KindVersion:            v.KindVersion,
			NodeImageVersion:       v.NodeImageVersion,
			UseNewContainerdConfig: kindUsesNewContainerdConfig(v.KindVersion),
		}
	}
}

// kindUsesNewContainerdConfig reports whether the given kind version uses containerd >= 2.x,
// which requires a different registry mirror config format. kind v0.27.0 was the first release
// to ship containerd 2.x.
func kindUsesNewContainerdConfig(kindVersion string) bool {
	v, err := semver.NewVersion(kindVersion)
	if err != nil {
		return false
	}
	threshold, _ := semver.NewVersion("0.27.0")
	return !v.LessThan(threshold)
}

// GetKindVersionConfig returns the kind version and the kind node image to use based on kubernetes version
func GetKindVersionConfig(kubeVersion string) (*KindConfig, error) {
	kubeSemVer, err := semver.NewVersion(kubeVersion)
	if err != nil {
		return nil, err
	}

	kindVersionConfig, found := kubeToKindVersion[fmt.Sprintf("%d.%d", kubeSemVer.Major(), kubeSemVer.Minor())]
	if !found {
		return nil, fmt.Errorf("unsupported kubernetes version. Supported versions are %s", strings.Join(kubeSupportedVersions(), ", "))
	}

	return &kindVersionConfig, nil
}

// kubeSupportedVersions returns a comma-separated list of supported kubernetes versions
func kubeSupportedVersions() []string {
	versions := make([]string, 0, len(kubeToKindVersion))

	for kubeVersion := range kubeToKindVersion {
		versions = append(versions, kubeVersion)
	}

	return versions
}

// generateKindConfig generates a kind cluster configuration based on the provided flags.
// the embed.FS should contain the kind-cluster.yaml template file.
func generateKindConfig(kindClusterTemplateFs embed.FS, flags KindConfigFlags) (string, error) {
	tmpl, err := template.ParseFS(kindClusterTemplateFs, "kind-cluster.yaml")
	if err != nil {
		return "", err
	}

	var kindClusterConfig strings.Builder
	if err = tmpl.Execute(&kindClusterConfig, flags); err != nil {
		return "", err
	}

	return kindClusterConfig.String(), nil
}
