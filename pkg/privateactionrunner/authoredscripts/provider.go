// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/artifacts"
)

// NewStaticProvider provides pre-downloaded authored-script artifacts from
// <root>/<digest>/<os>/<arch>. Each platform directory must contain a matching
// .artifact-digest marker written after the artifact is fully published.
// TODO: Remove this provider when Fleet's artifact provider is available to PAR.
func NewStaticProvider(rootDirectory string) artifacts.Provider {
	digestDirectory := strings.ReplaceAll(helmAddRepoDigest, ":", "-")
	platformDirectories := make(map[artifacts.Platform]string)
	for _, platform := range staticPlatforms() {
		platformDirectories[platform] = filepath.Join(rootDirectory, digestDirectory, platform.OS, platform.Arch)
	}
	return artifacts.NewLocalProvider(map[string]map[artifacts.Platform]string{
		helmAddRepoDigest: platformDirectories,
	})
}
