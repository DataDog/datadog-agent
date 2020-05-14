// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package flare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/docker/docker/api/types"
)

const dockerCommandMaxLength = 29

func zipDockerSelfInspect(tempDir, hostname string) error {
	du, err := docker.GetDockerUtil()
	if err != nil {
		return err
	}

	co, err := du.InspectSelf()
	if err != nil {
		return err
	}

	// Remove the envvars section, as we already
	// dump the whitelisted ones in envvars.log
	if co.Config != nil {
		co.Config.Env = []string{
			"Stripped out",
			"See runtime_config_dump.yaml for consolidated configuration",
			"and envvars.log for whitelisted envvars if found",
		}
	}

	// Serialise as JSON
	jsonStats, err := json.Marshal(co)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	json.Indent(&out, jsonStats, "", "\t") //nolint:errcheck
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
		fmt.Fprintf(w, "%s\t%s\t%q\t%s\t%v\t%v\t\n",
			c.ID[:12], c.Image, trimCommand(c.Command), c.Status, c.Ports, c.Names)
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

// trimCommand removes arguments from command string
// and trims it to 29 characters max.
func trimCommand(command string) string {
	cutoff := strings.Index(command, " ")
	if cutoff > 0 {
		// Add a trailing space between cmd and … to
		// differentiate removed args vs max length
		cutoff++
	} else {
		cutoff = len(command)
	}
	if cutoff > dockerCommandMaxLength {
		cutoff = dockerCommandMaxLength
	}

	if cutoff == len(command) {
		return command
	}
	return command[:cutoff] + "…"
}
