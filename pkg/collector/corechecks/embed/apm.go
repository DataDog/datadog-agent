// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build apm

package embed

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"

	log "github.com/cihub/seelog"
	"gopkg.in/yaml.v2"
)

type apmCheckConf struct {
	BinPath  string `yaml:"bin_path,omitempty"`
	ConfPath string `yaml:"conf_path,omitempty"`
}

// APMCheck keeps track of the running command
type APMCheck struct {
	cmd *exec.Cmd
}

func (c *APMCheck) String() string {
	return "APM Agent"
}

// Run executes the check
func (c *APMCheck) Run() error {
	// forward the standard output to the Agent logger
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			log.Info(in.Text())
		}
	}()

	// forward the standard error to the Agent logger
	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			log.Error(in.Text())
		}
	}()

	if err = c.cmd.Start(); err != nil {
		return err
	}

	return c.cmd.Wait()
}

// Configure the APMCheck
func (c *APMCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// handle the case when apm agent is disabled via the old `datadog.conf` file
	if enabled := config.Datadog.GetBool("apm_enabled"); !enabled {
		return fmt.Errorf("APM agent disabled through main configuration file")
	}

	var checkConf apmCheckConf
	if err := yaml.Unmarshal(data, &checkConf); err != nil {
		return err
	}

	binPath := ""
	defaultBinPath, defaultBinPathErr := getAPMAgentDefaultBinPath()
	if checkConf.BinPath != "" {
		if _, err := os.Stat(checkConf.BinPath); err == nil {
			binPath = checkConf.BinPath
		} else {
			log.Warnf("Can't access apm binary at %s, falling back to default path at %s", checkConf.BinPath, defaultBinPath)
		}
	}

	if binPath == "" {
		if defaultBinPathErr != nil {
			return defaultBinPathErr
		}

		binPath = defaultBinPath
	}

	// let the trace-agent use its own default config file if we haven't explicitly configured one
	ddConfigOption := ""
	if checkConf.ConfPath != "" {
		ddConfigOption = fmt.Sprintf("-ddconfig=%s", checkConf.ConfPath)
	}

	c.cmd = exec.Command(binPath, ddConfigOption)

	env := os.Environ()
	env = append(env, fmt.Sprintf("DD_HOSTNAME=%s", getHostname()))
	c.cmd.Env = env

	return nil
}

// Interval returns the scheduling time for the check, this will be scheduled only once
// since `Run` won't return, thus implementing a long running check.
func (c *APMCheck) Interval() time.Duration {
	return 0
}

// ID returns the name of the check since there should be only one instance running
func (c *APMCheck) ID() check.ID {
	return "APM_AGENT"
}

// Stop sends a termination signal to the APM process
func (c *APMCheck) Stop() {
	err := c.cmd.Process.Signal(os.Kill)
	if err != nil {
		log.Errorf("unable to stop APM check: %s", err)
	}
}

// GetWarnings does not return anything in APM
func (c *APMCheck) GetWarnings() []error {
	return []error{}
}

// GetMetricStats returns the stats from the last run of the check, but there aren't any
func (c *APMCheck) GetMetricStats() (map[string]int64, error) {
	return make(map[string]int64), nil
}

func init() {
	factory := func() check.Check {
		return &APMCheck{}
	}
	core.RegisterCheck("apm", factory)
}

func getHostname() string {
	hname, found := cache.Cache.Get(cache.BuildAgentKey("hostname"))
	if found {
		return hname.(string)
	}
	return ""
}
