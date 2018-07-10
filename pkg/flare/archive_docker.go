// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package flare

import (
	"bytes"
	json "github.com/json-iterator/go"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"text/tabwriter"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/docker/docker/api/types"
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

	w.RegisterReplacer(log.Replacer{
		Regex: regexp.MustCompile(`\"Image\": \"sha256:\w+"`),
		ReplFunc: func(s []byte) []byte {
			m := string(s[10 : len(s)-1])
			shaResolvedInspect, _ := du.ResolveImageName(m)
			return []byte(shaResolvedInspect)
		},
	})

	_, err = w.Write(serialized)
	return err
}

func zipDockerPs(tempDir, hostname string) error {
	du, err := docker.GetDockerUtil()
	if err != nil {
		// if we can't reach docker, let's do nothing
		log.Debugf("Couldn't reach docker for getting `docker ps`: %s", err)
		return nil
	}
	options := types.ContainerListOptions{All: true, Limit: 500}
	containerList, err := du.RawContainerList(options)
	if err != nil {
		return err
	}

	// Prepare contents
	var output bytes.Buffer
	w := tabwriter.NewWriter(&output, 20, 0, 3, ' ', tabwriter.AlignRight)

	fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tSTATUS\tPORTS\tNAMES\t")
	for _, c := range containerList {
		// Trimming command if too large
		var command_limit = 18
		command := c.Command
		if len(c.Command) >= command_limit {
			command = c.Command[:command_limit] + "…"
		}
		fmt.Fprintf(w, "%s\t%s\t%q\t%s\t%v\t%v\t\n",
			c.ID[:12], c.Image, command, c.Status, c.Ports, c.Names)
	}
	err = w.Flush()
	if err != nil {
		return err
	}

	// Write to file
	f := filepath.Join(tempDir, hostname, "docker_ps.log")
	file, err := NewRedactingWriter(f, os.ModePerm, false)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(output.Bytes())
	if err != nil {
		return err
	}

	return nil
}
