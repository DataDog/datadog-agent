// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package legacy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/docker"
	"github.com/DataDog/datadog-agent/pkg/config"

	yaml "gopkg.in/yaml.v2"
)

const (
	excludeIncludeWarn string = "\t- Warning: \"%s\" list now only " +
		"accepts prefix \"image:\" for image name or \"name:\" for container name. Prefix '%s' will be dropped. See " +
		"docker.yaml.example for more information\n"
	excludeIncludeBadFormat string = "\t- Warning: \"%s\" item wrongly formatted (should be 'tag:value'), got '%s'. Dropping it\n"

	warningNewCheck string = "Warning: the new docker check includes a change in the exclude/include list that may impact your billing. Please check docs/agent/changes.md#docker-check documentation to learn more."
)

func handleFilterList(input []string, name string) []string {
	list := []string{}
	for _, filter := range input {
		items := strings.SplitN(filter, ":", 2)
		if len(items) != 2 {
			fmt.Printf(excludeIncludeBadFormat, name, filter)
			continue
		}

		prefix := items[0]
		value := items[1]
		if prefix == "docker_image" || prefix == "image_name" || prefix == "image" {
			list = append(list, "image:"+value)
		} else if prefix == "container_name" || prefix == "name" {
			list = append(list, "name:"+value)
		} else {
			fmt.Printf(excludeIncludeWarn, name, prefix)
		}
	}
	return list
}

type legacyDockerInitConfig struct {
	DockerRoot string `yaml:"docker_root"`
}

type legacyDockerInstance struct {
	LabelAsTags []string `yaml:"collect_labels_as_tags"`
	Exclude     []string `yaml:"exclude"`
	Include     []string `yaml:"include"`
}

// ImportDockerConf read the configuration from docker_daemon check (agent5)
// and create the configuration for the new docker check (agent 6) and move
// needed option to datadog.yaml
func ImportDockerConf(src, dst string, overwrite bool, converter *config.LegacyConfigConverter) error {
	fmt.Printf("%s\n", warningNewCheck)

	// read docker_daemon.yaml
	c, err := providers.GetIntegrationConfigFromFile("docker_daemon", src)
	if err != nil {
		return fmt.Errorf("Could not load %s: %s", src, err)
	}

	if c.InitConfig != nil {
		initConf := &legacyDockerInitConfig{}
		if err := yaml.Unmarshal(c.InitConfig, initConf); err != nil {
			return fmt.Errorf("Could not Unmarshal init_config from %s: %s", src, err)
		}

		if initConf.DockerRoot != "" {
			converter.Set("container_cgroup_root", filepath.Join(initConf.DockerRoot, "sys", "fs", "cgroup"))
			converter.Set("container_proc_root", filepath.Join(initConf.DockerRoot, "proc"))
		}
	}

	if len(c.Instances) == 0 {
		return nil
	}
	if len(c.Instances) > 1 {
		fmt.Printf("Warning: %s contains more than one instance: converting only the first one\n", src)
	}

	dc := docker.DockerConfig{}
	if err := dc.Parse([]byte(c.Instances[0])); err != nil {
		return fmt.Errorf("Could not parse instance from %s: %s", src, err)
	}

	// write docker.yaml
	newCfg := map[string][]*docker.DockerConfig{
		"instances": {&dc},
	}

	data, err := yaml.Marshal(newCfg)
	if err != nil {
		return fmt.Errorf("Could not marshall final configuration for the new docker check: %s", err)
	}

	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		if overwrite {
			// we'll overwrite, backup the original file first
			err = os.Rename(dst, dst+".bak")
			if err != nil {
				return fmt.Errorf("unable to create a backup copy of the destination file: %v", err)
			}
		} else {
			return fmt.Errorf("destination file already exists, run the command again with --force or -f to overwrite it")
		}
	}
	// Create necessary destination dir
	err = os.MkdirAll(filepath.Dir(dst), 0750)
	if err != nil {
		return err
	}

	if err := os.WriteFile(dst, data, 0640); err != nil {
		return fmt.Errorf("Could not write new docker configuration to %s: %s", dst, err)
	}

	fmt.Printf("Successfully imported the contents of %s into %s\n", src, dst)

	instance := &legacyDockerInstance{}
	if err := yaml.Unmarshal(c.Instances[0], instance); err != nil {
		return fmt.Errorf("Could not Unmarshal instances from %s: %s", src, err)
	}

	// filter include/exclude list
	if acExclude := handleFilterList(instance.Exclude, "exclude"); len(acExclude) != 0 {
		converter.Set("ac_exclude", acExclude)
	}

	if acInclude := handleFilterList(instance.Include, "include"); len(acInclude) != 0 {
		converter.Set("ac_include", acInclude)
	}

	// move 'collect_labels_as_tags' to 'docker_labels_as_tags'
	if len(instance.LabelAsTags) != 0 {
		dockerLabelAsTags := map[string]string{}
		for _, label := range instance.LabelAsTags {
			dockerLabelAsTags[label] = label
		}
		converter.Set("docker_labels_as_tags", dockerLabelAsTags)
	}

	fmt.Printf("Successfully imported the contents of %s into datadog.yaml (see 'Autodiscovery' section in datadog.yaml.example)\n\n", src)
	return nil
}
