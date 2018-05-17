// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package jmx

import (
	"fmt"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	"gopkg.in/yaml.v2"
)

var runner *jmxfetch.JMXFetch = &jmxfetch.JMXFetch{}

// checkInstanceCfg lists the config options on the instance against which we make some sanity checks
// on how they're configured. All the other options should be checked on JMXFetch's side.
type checkInstanceCfg struct {
	JavaBinPath      string `yaml:"java_bin_path,omitempty"`
	JavaOptions      string `yaml:"java_options,omitempty"`
	ToolsJarPath     string `yaml:"tools_jar_path,omitempty"`
	ProcessNameRegex string `yaml:"process_name_regex,omitempty"`
}

type checkInitCfg struct {
	CustomJarPaths []string `yaml:"custom_jar_paths,omitempty"`
	ToolsJarPath   string   `yaml:"tools_jar_path,omitempty"`
	JavaBinPath    string   `yaml:"java_bin_path,omitempty"`
	JavaOptions    string   `yaml:"java_options,omitempty"`
}

const windowsExitFile = "jmxfetch_exit"

func initRunner() {
	runner = &jmxfetch.JMXFetch{}
	runner.LogLevel = config.Datadog.GetString("log_level")

	if runtime.GOOS == "windows" {
		runner.JmxExitFile = windowsExitFile
	}
}

func startRunner() error {
	err := runner.Start()
	if err != nil {
		return err
	}
	return nil
}

func configureRunner(instance, initConfig integration.Data) error {

	var initConf checkInitCfg
	var instanceConf checkInstanceCfg

	// unmarshall instance info
	if err := yaml.Unmarshal(instance, &instanceConf); err != nil {
		return err
	}

	// unmarshall init config
	if err := yaml.Unmarshal(initConfig, &initConf); err != nil {
		return err
	}

	if runner.JavaBinPath == "" {
		if instanceConf.JavaBinPath != "" {
			runner.JavaBinPath = instanceConf.JavaBinPath
		} else if initConf.JavaBinPath != "" {
			runner.JavaBinPath = initConf.JavaBinPath
		}
	}
	if runner.JavaOptions == "" {
		if instanceConf.JavaOptions != "" {
			runner.JavaOptions = instanceConf.JavaOptions
		} else if initConf.JavaOptions != "" {
			runner.JavaOptions = initConf.JavaOptions
		}
	}
	if runner.JavaToolsJarPath == "" {
		if instanceConf.ToolsJarPath != "" {
			runner.JavaToolsJarPath = instanceConf.ToolsJarPath
		} else if initConf.ToolsJarPath != "" {
			runner.JavaToolsJarPath = initConf.ToolsJarPath
		}
	}
	if runner.JavaCustomJarPaths == nil {
		if initConf.CustomJarPaths != nil {
			runner.JavaCustomJarPaths = initConf.CustomJarPaths
		}
	}

	if instanceConf.ProcessNameRegex != "" {
		if runner.JavaToolsJarPath == "" {
			return fmt.Errorf("You must specify the path to tools.jar. See http://docs.datadoghq.com/integrations/java/ for more information")
		}
	}

	return nil
}
