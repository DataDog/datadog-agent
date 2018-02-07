// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package flare

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

func zipDockerSelfInspect(tempDir, hostname string) error {
	du, err := docker.GetDockerUtil()
	if err != nil {
		return err
	}

	co, err := du.InspectSelf()
	if err != nil {
		return err
	}

	// Serialise as JSON
	jsonStats, err := json.Marshal(co)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	json.Indent(&out, jsonStats, "", "\t")
	serialized := out.Bytes()

	// Clean it up
	cleaned, err := credentialsCleanerBytes(serialized)
	if err != nil {
		return err
	}

	imageSha := regexp.MustCompile(`\"Image\": \"sha256:\w+"`)
	cleaned = imageSha.ReplaceAllFunc(cleaned, func(s []byte) []byte {
		m := string(s[10 : len(s)-1])
		shaResolvedInspect, _ := du.ResolveImageName(m)
		return []byte(shaResolvedInspect)
	})

	f := filepath.Join(tempDir, hostname, "docker_inspect.log")

	err = os.MkdirAll(filepath.Dir(f), os.ModePerm)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(f, cleaned, os.ModePerm)
	if err != nil {
		return err
	}

	return err
}
