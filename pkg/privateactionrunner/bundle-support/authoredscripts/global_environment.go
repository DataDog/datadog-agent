// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"fmt"
	"os"
	"path/filepath"

	securejoin "github.com/cyphar/filepath-securejoin"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/artifacts"
)

const (
	datadogAgentCacheDirectory    = "datadog-agent"
	authoredScriptsCacheDirectory = "authored-scripts"
	globalStateDirectory          = "state"
)

func globalEnvironmentDirectory(artifactDigest string) (string, error) {
	userCacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("could not locate the OS user cache for authored-script global state: %w", err)
	}
	digestDirectory, err := artifacts.DigestPathComponent(artifactDigest)
	if err != nil {
		return "", err
	}
	directory, err := securejoin.SecureJoin(userCacheDirectory, filepath.Join(
		datadogAgentCacheDirectory,
		authoredScriptsCacheDirectory,
		globalStateDirectory,
		digestDirectory,
	))
	if err != nil {
		return "", fmt.Errorf("could not resolve authored-script global state directory: %w", err)
	}
	if err := os.MkdirAll(directory, sessionDirectoryMode); err != nil {
		return "", fmt.Errorf("could not create authored-script global state directory %q: %w", directory, err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", fmt.Errorf("could not access authored-script global state directory %q: %w", directory, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("authored-script global state path %q is not a directory", directory)
	}
	return directory, nil
}
