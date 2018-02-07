// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package embed

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	log "github.com/cihub/seelog"
	"gopkg.in/yaml.v2"
)

const (
	linkToDoc = "See http://docs.datadoghq.com/integrations/java/ for more information"
)

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

// JMXCheck keeps track of the running command
type JMXCheck struct {
	runner      *jmxfetch.JMXFetch
	checks      map[string]struct{}
	isAttachAPI bool
	running     uint32
	stop        chan struct{}
	stopDone    chan struct{}
}

var jmxLauncher = JMXCheck{
	checks:   make(map[string]struct{}),
	stop:     make(chan struct{}),
	stopDone: make(chan struct{}),
	runner:   jmxfetch.New(jmxExitFile),
}

func (c *JMXCheck) String() string {
	return "JMX Check"
}

// Run executes the check with retries
func (c *JMXCheck) Run() error {
	if len(c.checks) == 0 {
		return fmt.Errorf("No JMX checks configured - skipping.")
	}

	atomic.StoreUint32(&c.running, 1)
	// TODO: retries should be configurable with meaningful default values
	err := check.Retry(defaultRetryDuration, defaultRetries, c.run, c.String())
	atomic.StoreUint32(&c.running, 0)

	return err
}

// run executes the check
func (c *JMXCheck) run() error {
	select {
	// poll the stop channel once to make sure no stop was requested since the last call to `run`
	case <-c.stop:
		log.Info("Not starting JMX: stop requested")
		c.stopDone <- struct{}{}
		return nil
	default:
	}

	c.runner.LogLevel = config.Datadog.GetString("log_level")

	err := c.runner.Run()
	if err != nil {
		return retryExitError(err)
	}

	processDone := make(chan error)
	go func() {
		processDone <- c.runner.Wait()
	}()

	select {
	case err = <-processDone:
		return retryExitError(err)
	case <-c.stop:
		err = c.runner.Kill()
		if err != nil {
			log.Errorf("unable to stop JMX check: %s", err)
		}
	}

	// wait for process to exit
	err = <-processDone
	c.stopDone <- struct{}{}
	return err
}

func (c *JMXCheck) Parse(data, initConfig check.ConfigData) error {

	var initConf checkInitCfg
	var instanceConf checkInstanceCfg

	// unmarshall instance info
	if err := yaml.Unmarshal(data, &instanceConf); err != nil {
		return err
	}

	// unmarshall init config
	if err := yaml.Unmarshal(initConfig, &initConf); err != nil {
		return err
	}

	if c.runner.JavaBinPath == "" {
		if instanceConf.JavaBinPath != "" {
			c.runner.JavaBinPath = instanceConf.JavaBinPath
		} else if initConf.JavaBinPath != "" {
			c.runner.JavaBinPath = initConf.JavaBinPath
		}
	}
	if c.runner.JavaOptions == "" {
		if instanceConf.JavaOptions != "" {
			c.runner.JavaOptions = instanceConf.JavaOptions
		} else if initConf.JavaOptions != "" {
			c.runner.JavaOptions = initConf.JavaOptions
		}
	}
	if c.runner.JavaToolsJarPath == "" {
		if instanceConf.ToolsJarPath != "" {
			c.runner.JavaToolsJarPath = instanceConf.ToolsJarPath
		} else if initConf.ToolsJarPath != "" {
			c.runner.JavaToolsJarPath = initConf.ToolsJarPath
		}
	}
	if c.runner.JavaCustomJarPaths == nil {
		if initConf.CustomJarPaths != nil {
			c.runner.JavaCustomJarPaths = initConf.CustomJarPaths
		}
	}

	if instanceConf.ProcessNameRegex != "" {
		if c.runner.JavaToolsJarPath == "" {
			return fmt.Errorf("You must specify the path to tools.jar. %s", linkToDoc)
		}
		c.isAttachAPI = true
	}

	return nil
}

// Configure the JMXCheck
func (c *JMXCheck) Configure(data, initConfig check.ConfigData) error {
	return c.Parse(data, initConfig)
}

// Interval returns the scheduling time for the check, this will be scheduled only once
// since `Run` won't return, thus implementing a long running check.
func (c *JMXCheck) Interval() time.Duration {
	return 0
}

// ID returns the name of the check since there should be only one instance running
func (c *JMXCheck) ID() check.ID {
	return "JMX_Check"
}

// GetMetricStats returns the stats from the last run of the check, but there aren't any yet
func (c *JMXCheck) GetMetricStats() (map[string]int64, error) {
	return make(map[string]int64), nil
}

// Stop sends a termination signal to the JMXFetch process
func (c *JMXCheck) Stop() {
	if atomic.LoadUint32(&c.running) == 0 {
		log.Info("JMX not running.")
		return
	}

	c.stop <- struct{}{}
	<-c.stopDone
}

// GetWarnings does not return anything in JMX
func (c *JMXCheck) GetWarnings() []error {
	return []error{}
}
