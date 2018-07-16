// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build process
// +build darwin freebsd

package embed

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	enabled     bool
	binPath     string
	commandOpts []string
	running     uint32
	stop        chan struct{}
	stopDone    chan struct{}
}

func (c *ProcessAgentCheck) String() string {
	return "Process Agent"
}

func (c *ProcessAgentCheck) Version() string {
	return ""
}

// Run executes the check with retries
func (c *ProcessAgentCheck) Run() error {
	atomic.StoreUint32(&c.running, 1)
	// TODO: retries should be configurable with meaningful default values
	err := check.Retry(defaultRetryDuration, defaultRetries, c.run, c.String())
	atomic.StoreUint32(&c.running, 0)

	return err
}

// run executes the check
func (c *ProcessAgentCheck) run() error {
	if !c.enabled {
		log.Info("Not running process_agent because 'enabled' is false in init_config")
		return nil
	}

	select {
	// poll the stop channel once to make sure no stop was requested since the last call to `run`
	case <-c.stop:
		log.Info("Not starting Process Agent check: stop requested")
		c.stopDone <- struct{}{}
		return nil
	default:
	}

	cmd := exec.Command(c.binPath, c.commandOpts...)

	// forward the standard output to the Agent logger
	stdout, err := cmd.StdoutPipe()
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
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			log.Error(in.Text())
		}
	}()

	if err = cmd.Start(); err != nil {
		return retryExitError(err)
	}

	processDone := make(chan error)
	go func() {
		processDone <- cmd.Wait()
	}()

	select {
	case err = <-processDone:
		return retryExitError(err)
	case <-c.stop:
		err = cmd.Process.Signal(os.Kill)
		if err != nil {
			log.Errorf("unable to stop process-agent check: %s", err)
		}
	}

	// wait for process to exit
	err = <-processDone
	c.stopDone <- struct{}{}
	return err
}

// Configure the ProcessAgentCheck
func (c *ProcessAgentCheck) Configure(data integration.Data, initConfig integration.Data) error {
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

	c.binPath = ""
	defaultBinPath, defaultBinPathErr := getProcessAgentDefaultBinPath()
	if checkConf.BinPath != "" {
		if _, err := os.Stat(checkConf.BinPath); err == nil {
			c.binPath = checkConf.BinPath
		} else {
			log.Warnf("Can't access process-agent binary at %s, falling back to default path at %s", checkConf.BinPath, defaultBinPath)
		}
	}

	if c.binPath == "" {
		if defaultBinPathErr != nil {
			return defaultBinPathErr
		}
		c.binPath = defaultBinPath
	}

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
	if atomic.LoadUint32(&c.running) == 0 {
		log.Info("Process Agent not running.")
		return
	}

	c.stop <- struct{}{}
	<-c.stopDone
}

// GetMetricStats returns the stats from the last run of the check, but there aren't any yet
func (c *ProcessAgentCheck) GetMetricStats() (map[string]int64, error) {
	return make(map[string]int64), nil
}

func init() {
	factory := func() check.Check {
		return &ProcessAgentCheck{
			stop:     make(chan struct{}),
			stopDone: make(chan struct{}),
		}
	}
	core.RegisterCheck("process_agent", factory)
}

func getProcessAgentDefaultBinPath() (string, error) {
	here, _ := executable.Folder()
	binPath := filepath.Join(here, "..", "..", "embedded", "bin", "process-agent")
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}
	return binPath, fmt.Errorf("Can't access the default process-agent binary at %s", binPath)
}

// GetWarnings does not return anything
func (c *ProcessAgentCheck) GetWarnings() []error {
	return []error{}
}
