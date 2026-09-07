// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package components

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// GenerateAndDownloadAgentFlare builds an agent flare on the host and downloads it to outputDir.
func GenerateAndDownloadAgentFlare(agent *RemoteHostAgent, host *RemoteHost, outputDir string) (string, error) {
	if agent == nil || host == nil {
		return "", errors.New("Agent or RemoteHost component is not initialized, cannot generate flare")
	}
	// generate a flare, it will fallback to local flare generation if the running agent cannot be reached
	// todo skip uploading it to backend, requires further changes in agent executor
	// to redirect stdin to null, on linux adding `</dev/null`
	// on windows prepending command with `@() |`, pre-piping with an empty array
	// discard error, flare command might return error if there is no intake, but it the archive is still generated
	// --keep-archive prevents the agent from deleting the local archive after a successful upload,
	// since we need it to still be on disk afterwards to download it below.
	flareCommandOutput, err := agent.Client.FlareWithError(agentclient.WithArgs([]string{"--email", "e2e-tests@datadog-agent", "--send", "--keep-archive"}))

	lines := []string{flareCommandOutput}
	if err != nil {
		lines = append(lines, err.Error())
	}
	// on error, the flare output is in the error message
	flareCommandOutput = strings.Join(lines, "\n")

	// find <path to flare>.zip in flare command output
	// (?m) is a flag that allows ^ and $ to match the beginning and end of each line
	re := regexp.MustCompile(`(?m)^(.+\.zip) is going to be uploaded to Datadog$`)
	matches := re.FindStringSubmatch(flareCommandOutput)
	if len(matches) < 2 {
		return "", fmt.Errorf("output does not contain the path to the flare archive, output: %s", flareCommandOutput)
	}
	flarePath := matches[1]
	flareFileInfo, err := host.Lstat(flarePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat flare archive: %w", err)
	}
	dstPath := filepath.Join(outputDir, flareFileInfo.Name())

	err = host.EnsureFileIsReadable(flarePath)
	if err != nil {
		return "", fmt.Errorf("failed to ensure flare archive is readable: %w", err)
	}
	err = host.GetFile(flarePath, dstPath)
	if err != nil {
		return "", fmt.Errorf("failed to download flare archive: %w", err)
	}
	return dstPath, nil
}
