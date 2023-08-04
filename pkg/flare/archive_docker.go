// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package flare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/docker/docker/api/types"
)

const dockerCommandMaxLength = 29

func getDockerSelfInspect() ([]byte, error) {
	if !config.IsContainerized() {
		return nil, fmt.Errorf("The Agent is not containerized")
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		return nil, err
	}

	selfContainerID, err := metrics.GetProvider().GetMetaCollector().GetSelfContainerID()
	if err != nil {
		return nil, fmt.Errorf("Unable to determine self container id, err: %w", err)
	}

	co, err := du.Inspect(context.TODO(), selfContainerID, false)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	var out bytes.Buffer
	json.Indent(&out, jsonStats, "", "\t") //nolint:errcheck
	serialized := out.Bytes()

	// replace all Image: sha256:xxx with a resolved image name
	imgRx := regexp.MustCompile(`\"Image\": \"sha256:\w+"`)
	replFunc := func(s []byte) []byte {
		m := string(s[10 : len(s)-1])
		shaResolvedInspect, _ := du.ResolveImageName(context.TODO(), m)
		return []byte(shaResolvedInspect)
	}
	serialized = imgRx.ReplaceAllFunc(serialized, replFunc)

	return serialized, nil
}

func getDockerPs() ([]byte, error) {
	du, err := docker.GetDockerUtil()
	if err != nil {
		// if we can't reach docker, let's do nothing
		log.Debugf("Couldn't reach docker for getting `docker ps`: %s", err)
		return nil, nil
	}
	options := types.ContainerListOptions{All: true, Limit: 500}
	containerList, err := du.RawContainerList(context.TODO(), options)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	// Write to file
	return output.Bytes(), nil
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
