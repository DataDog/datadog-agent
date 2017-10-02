// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package legacy

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/config"

	yaml "gopkg.in/yaml.v2"
)

const (
	excludeIncludeWarn string = "\t- Warning: \"%s\" list now only " +
		"accepts prefix \"image:\" for image name or \"name:\" for container name. Prefix '%s' will be dropped. See " +
		"docker.yaml.example for more information\n"
	excludeIncludeBadFormat string = "\t- Warning: \"%s\" item wrongly formatted (should be 'tag:value'), got '%s'. Dropping it\n"

	warningNewCheck string = "Warning: the new docker check includes a change in the exclude/include list that may impact your billing. Please check docs/beta/changes.md documentation to learn more."
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
func ImportDockerConf(src, dst string, overwrite bool) error {
	fmt.Printf("%s\n", warningNewCheck)

	// read docker_daemon.yaml
	c, err := providers.GetCheckConfigFromFile("docker_daemon", src)
	if err != nil {
		return fmt.Errorf("Could not load %s: %s", src, err)
	}

	if c.InitConfig != nil {
		initConf := &legacyDockerInitConfig{}
		if err := yaml.Unmarshal(c.InitConfig, initConf); err != nil {
			return fmt.Errorf("Could not Unmarshal init_config from %s: %s", src, err)
		}

		if initConf.DockerRoot != "" {
			config.Datadog.Set("container_cgroup_root", path.Join(initConf.DockerRoot, "sys", "fs", "cgroup"))
			config.Datadog.Set("container_proc_root", path.Join(initConf.DockerRoot, "proc"))
		}
	}

	if len(c.Instances) == 0 {
		return nil
	}
	if len(c.Instances) > 1 {
		fmt.Printf("Warning: %s contains more than one instance: converting only the first one", src)
	}

	dc := containers.DockerConfig{}
	if err := dc.Parse([]byte(c.Instances[0])); err != nil {
		return fmt.Errorf("Could not parse instance from %s: %s", src, err)
	}

	// write docker.yaml
	data, err := yaml.Marshal(dc)
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

	if err := ioutil.WriteFile(dst, data, 0640); err != nil {
		return fmt.Errorf("Could not write new docker configuration to %s: %s", dst, err)
	}

	fmt.Printf("Successfully imported the contents of %s into %s\n", src, dst)

	instance := &legacyDockerInstance{}
	if err := yaml.Unmarshal(c.Instances[0], instance); err != nil {
		return fmt.Errorf("Could not Unmarshal instances from %s: %s", src, err)
	}

	// filter include/exclude list
	if ac_exclude := handleFilterList(instance.Exclude, "exclude"); len(ac_exclude) != 0 {
		config.Datadog.Set("ac_exclude", ac_exclude)
	}

	if ac_include := handleFilterList(instance.Include, "include"); len(ac_include) != 0 {
		config.Datadog.Set("ac_include", ac_include)
	}

	// move 'collect_labels_as_tags' to 'docker_labels_as_tags'
	if len(instance.LabelAsTags) != 0 {
		dockerLabelAsTags := map[string]string{}
		for _, label := range instance.LabelAsTags {
			dockerLabelAsTags[label] = label
		}
		config.Datadog.Set("docker_labels_as_tags", dockerLabelAsTags)
	}

	fmt.Printf("Successfully move information needed from %s into the datadog.yaml (see 'Autodiscovery' section in datadog.yaml.example)\n\n", src)
	return nil
}
