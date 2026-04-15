// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rpm

import (
	"bufio"
	"fmt"
	"os"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// NewAmazonLinux2022Backend creates a new backend for Amazon Linux 2022
func NewAmazonLinux2022Backend(target *types.Target, reposDir string, logger types.Logger) (*RedHatBackend, error) {
	releaseVer, err := extractReleaseVersionFromImageID()
	if err != nil {
		return nil, fmt.Errorf("extract release version: %w", err)
	}

	b, err := dnfv2.NewBackend(releaseVer, reposDir)
	if err != nil {
		return nil, err
	}

	return &RedHatBackend{
		target:     target,
		logger:     logger,
		dnfBackend: b,
	}, nil
}

var imageFilePattern = regexp.MustCompile(`image_file="al2022-\w+-(2022.0.\d{8}).*"`)

func extractReleaseVersionFromImageID() (string, error) {
	imageIDPath := types.HostEtc("image-id")
	f, err := os.Open(imageIDPath)
	if err != nil {
		return "", err
	}

	liner := bufio.NewScanner(f)
	for liner.Scan() {
		if submatches := imageFilePattern.FindStringSubmatch(liner.Text()); submatches != nil {
			return submatches[1], nil
		}
	}

	if err := liner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("image_file entry not found in %s", imageIDPath)
}
