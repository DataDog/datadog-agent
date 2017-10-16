// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package flare

import (
	"path/filepath"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/jhoonb/archivex"
)

func zipDockerSelfInspect(zipFile *archivex.ZipFile, hostname string) error {

	co, err := docker.ContainerSelfInspect()
	if err != nil {
		return err
	}
	// Clean it up
	cleaned, err := credentialsCleanerBytes(co)
	if err != nil {
		return err
	}

	imageSha := regexp.MustCompile(`\"Image\": \"sha256:\w+"`)
	cleaned = imageSha.ReplaceAllFunc(cleaned, func(s []byte) []byte {
		m := string(s[10 : len(s)-1])
		shaResolvedInspect, _ := docker.ResolveImageName(m)
		return []byte(shaResolvedInspect)
	})

	err = zipFile.Add(filepath.Join(hostname, "docker_inspect.log"), cleaned)
	if err != nil {
		return err
	}
	return err
}
