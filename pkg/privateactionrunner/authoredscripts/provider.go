// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/artifacts"
)

const (
	datadogAgentCacheDirectory    = "datadog-agent"
	authoredScriptsCacheDirectory = "authored-scripts"
)

// NewStaticProvider provides pre-downloaded authored-script artifacts from
// the current user's OS cache. Each platform directory must contain a matching
// .artifact-digest marker written after the artifact is fully published.
// TODO: Remove this provider when Fleet's artifact provider is available to PAR.
func NewStaticProvider() (artifacts.Provider, error) {
	userCacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("could not locate the OS user cache: %w", err)
	}
	rootDirectory := filepath.Join(userCacheDirectory, datadogAgentCacheDirectory, authoredScriptsCacheDirectory)
	digestDirectory := strings.ReplaceAll(helmAddRepoDigest, ":", "-")
	platformDirectories := make(map[artifacts.Platform]string)
	for _, platform := range staticPlatforms() {
		platformDirectories[platform] = filepath.Join(rootDirectory, digestDirectory, platform.OS, platform.Arch)
	}
	return artifacts.NewLocalProvider(map[string]map[artifacts.Platform]string{
		helmAddRepoDigest: platformDirectories,
	}), nil
}
