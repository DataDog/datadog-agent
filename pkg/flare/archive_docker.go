// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package flare

import (
	"bytes"
	"encoding/json"
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

	f := filepath.Join(tempDir, hostname, "docker_inspect.log")
	w, err := NewRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	w.RegisterReplacer(replacer{
		regex: regexp.MustCompile(`\"Image\": \"sha256:\w+"`),
		replFunc: func(s []byte) []byte {
			m := string(s[10 : len(s)-1])
			shaResolvedInspect, _ := du.ResolveImageName(m)
			return []byte(shaResolvedInspect)
		},
	})

	_, err = w.Write(serialized)
	return err
}
