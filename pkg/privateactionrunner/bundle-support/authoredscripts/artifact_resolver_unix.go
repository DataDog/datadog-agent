// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"fmt"
	"os"
	"path/filepath"
)

const datadogAgentCacheDirectory = "datadog-agent"

func defaultArtifactStoreRoot() (string, error) {
	userCacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("could not locate the OS user cache: %w", err)
	}
	return filepath.Join(userCacheDirectory, datadogAgentCacheDirectory, authoredScriptCacheDirectory), nil
}
