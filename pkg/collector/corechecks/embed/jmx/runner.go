// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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

type runner struct {
	jmxfetch *jmxfetch.JMXFetch
	started  bool
}

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

func (r *runner) initRunner() {
	r.jmxfetch = &jmxfetch.JMXFetch{}
	r.jmxfetch.LogLevel = config.Datadog.GetString("log_level")
}

func (r *runner) startRunner() error {

	lifecycleMgmt := true
	if runtime.GOOS == "windows" {
		lifecycleMgmt = false
	}

	err := r.jmxfetch.Start(lifecycleMgmt)
	if err != nil {
		return err
	}
	r.started = true
	return nil
}

func (r *runner) configureRunner(instance, initConfig integration.Data) error {

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

	if r.jmxfetch.JavaBinPath == "" {
		if instanceConf.JavaBinPath != "" {
			r.jmxfetch.JavaBinPath = instanceConf.JavaBinPath
		} else if initConf.JavaBinPath != "" {
			r.jmxfetch.JavaBinPath = initConf.JavaBinPath
		}
	}
	if r.jmxfetch.JavaOptions == "" {
		if instanceConf.JavaOptions != "" {
			r.jmxfetch.JavaOptions = instanceConf.JavaOptions
		} else if initConf.JavaOptions != "" {
			r.jmxfetch.JavaOptions = initConf.JavaOptions
		}
	}
	if r.jmxfetch.JavaToolsJarPath == "" {
		if instanceConf.ToolsJarPath != "" {
			r.jmxfetch.JavaToolsJarPath = instanceConf.ToolsJarPath
		} else if initConf.ToolsJarPath != "" {
			r.jmxfetch.JavaToolsJarPath = initConf.ToolsJarPath
		}
	}
	if r.jmxfetch.JavaCustomJarPaths == nil {
		if initConf.CustomJarPaths != nil {
			r.jmxfetch.JavaCustomJarPaths = initConf.CustomJarPaths
		}
	}

	if instanceConf.ProcessNameRegex != "" {
		if r.jmxfetch.JavaToolsJarPath == "" {
			return fmt.Errorf("You must specify the path to tools.jar. See http://docs.datadoghq.com/integrations/java/ for more information")
		}
	}

	return nil
}

func (r *runner) stopRunner() error {
	if r.jmxfetch != nil && r.started {
		return r.jmxfetch.Stop()
	}
	return nil
}
