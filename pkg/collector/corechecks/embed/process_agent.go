// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build process

package embed

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"

	log "github.com/cihub/seelog"
	"github.com/kardianos/osext"
	"gopkg.in/yaml.v2"
)

type processAgentInitConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
}

type processAgentCheckConf struct {
	BinPath  string `yaml:"bin_path,omitempty"`
	ConfPath string `yaml:"conf_path,omitempty"`
}

// ProcessAgentCheck keeps track of the running command
type ProcessAgentCheck struct {
	enabled bool
	cmd     *exec.Cmd
}

func (c *ProcessAgentCheck) String() string {
	return "Process Agent"
}

// Run executes the check
func (c *ProcessAgentCheck) Run() error {
	if !c.enabled {
		log.Info("Not running process_agent because 'enabled' is false in init_config")
		return nil
	}

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

// Configure the ProcessAgentCheck
func (c *ProcessAgentCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// handle the case when the agent is disabled via the old `datadog.conf` file
	if enabled := config.Datadog.GetBool("process_agent_enabled"); !enabled {
		return fmt.Errorf("Process Agent disabled through main configuration file")
	}

	var initConf processAgentInitConfig
	if err := yaml.Unmarshal(initConfig, &initConf); err != nil {
		return err
	}
	c.enabled = initConf.Enabled

	var checkConf processAgentCheckConf
	if err := yaml.Unmarshal(data, &checkConf); err != nil {
		return err
	}

	binPath := ""
	defaultBinPath, defaultBinPathErr := getProcessAgentDefaultBinPath()
	if checkConf.BinPath != "" {
		if _, err := os.Stat(checkConf.BinPath); err == nil {
			binPath = checkConf.BinPath
		} else {
			log.Warnf("Can't access process-agent binary at %s, falling back to default path at %s", checkConf.BinPath, defaultBinPath)
		}
	}

	if binPath == "" {
		if defaultBinPathErr != nil {
			return defaultBinPathErr
		}
		binPath = defaultBinPath
	}

	// let the process agent use its own config file provided by the Agent package
	// if we haven't found one in the process-agent.yaml check config
	configFile := checkConf.ConfPath
	if configFile == "" {
		configFile = path.Join(config.FileUsedDir(), "process-agent.ini")
	}

	c.cmd = exec.Command(binPath, fmt.Sprintf("-ddconfig=%s", configFile))

	env := os.Environ()
	env = append(env, fmt.Sprintf("DD_API_KEY=%s", config.Datadog.GetString("api_key")))
	env = append(env, fmt.Sprintf("DD_HOSTNAME=%s", getHostname()))
	c.cmd.Env = env

	return nil
}

// InitSender initializes a sender but we don't need any
func (c *ProcessAgentCheck) InitSender() {}

// Interval returns the scheduling time for the check, this will be scheduled only once
// since `Run` won't return, thus implementing a long running check.
func (c *ProcessAgentCheck) Interval() time.Duration {
	return 0
}

// ID returns the name of the check since there should be only one instance running
func (c *ProcessAgentCheck) ID() check.ID {
	return "PROCESS_AGENT"
}

// Stop sends a termination signal to the process-agent process
func (c *ProcessAgentCheck) Stop() {
	if !c.enabled {
		return
	}

	err := c.cmd.Process.Signal(os.Kill)
	if err != nil {
		log.Errorf("unable to stop process-agent check: %s", err)
	}
}

// GetMetricStats returns the stats from the last run of the check, but there aren't any yet
func (c *ProcessAgentCheck) GetMetricStats() (map[string]int64, error) {
	return make(map[string]int64), nil
}

func init() {
	factory := func() check.Check {
		return &ProcessAgentCheck{}
	}
	core.RegisterCheck("process_agent", factory)
}

func getProcessAgentDefaultBinPath() (string, error) {
	here, _ := osext.ExecutableFolder()
	binPath := path.Join(here, "..", "..", "embedded", "bin", "process-agent")
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}
	return binPath, fmt.Errorf("Can't access the default process-agent binary at %s", binPath)
}

// GetWarnings does not return anything
func (c *ProcessAgentCheck) GetWarnings() []error {
	return []error{}
}
