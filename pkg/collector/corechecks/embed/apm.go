// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build apm
// +build !windows
// +build !linux

package embed

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	yaml "gopkg.in/yaml.v2"
)

type apmCheckConf struct {
	BinPath string `yaml:"bin_path,omitempty"`
}

// APMCheck keeps track of the running command
type APMCheck struct {
	binPath     string
	commandOpts []string
	running     uint32
	stop        chan struct{}
	stopDone    chan struct{}
	source      string
	telemetry   bool
}

func (c *APMCheck) String() string {
	return "APM Agent"
}

func (c *APMCheck) Version() string {
	return ""
}

func (c *APMCheck) ConfigSource() string {
	return c.source
}

// Run executes the check with retries
func (c *APMCheck) Run() error {
	atomic.StoreUint32(&c.running, 1)
	// TODO: retries should be configurable with meaningful default values
	err := check.Retry(defaultRetryDuration, defaultRetries, c.run, c.String())
	atomic.StoreUint32(&c.running, 0)

	return err
}

// run executes the check
func (c *APMCheck) run() error {
	select {
	// poll the stop channel once to make sure no stop was requested since the last call to `run`
	case <-c.stop:
		log.Info("Not starting APM check: stop requested")
		c.stopDone <- struct{}{}
		return nil
	default:
	}

	cmd := exec.Command(c.binPath, c.commandOpts...)

	env := os.Environ()
	env = append(env, fmt.Sprintf("DD_API_KEY=%s", config.Datadog.GetString("api_key")))
	env = append(env, fmt.Sprintf("DD_HOSTNAME=%s", getHostname()))
	env = append(env, fmt.Sprintf("DD_DOGSTATSD_PORT=%s", config.Datadog.GetString("dogstatsd_port")))
	env = append(env, fmt.Sprintf("DD_LOG_LEVEL=%s", config.Datadog.GetString("log_level")))
	cmd.Env = env

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
			log.Errorf("unable to stop APM check: %s", err)
		}
	}

	// wait for process to exit
	err = <-processDone
	c.stopDone <- struct{}{}
	return err
}

// Configure the APMCheck
func (c *APMCheck) Configure(data integration.Data, initConfig integration.Data, source string) error {
	var checkConf apmCheckConf
	if err := yaml.Unmarshal(data, &checkConf); err != nil {
		return err
	}

	c.binPath = ""
	defaultBinPath, defaultBinPathErr := getAPMAgentDefaultBinPath()
	if checkConf.BinPath != "" {
		if _, err := os.Stat(checkConf.BinPath); err == nil {
			c.binPath = checkConf.BinPath
		} else {
			log.Warnf("Can't access apm binary at %s, falling back to default path at %s", checkConf.BinPath, defaultBinPath)
		}
	}

	if c.binPath == "" {
		if defaultBinPathErr != nil {
			return defaultBinPathErr
		}

		c.binPath = defaultBinPath
	}

	configFile := config.Datadog.ConfigFileUsed()

	c.commandOpts = []string{}

	// explicitly provide to the trace-agent the agent configuration file
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		c.commandOpts = append(c.commandOpts, fmt.Sprintf("-config=%s", configFile))
	}

	c.source = source
	c.telemetry = telemetry.IsCheckEnabled("apm")
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

// IsTelemetryEnabled returns if the telemetry is enabled for this check
func (c *APMCheck) IsTelemetryEnabled() bool {
	return c.telemetry
}

// Stop sends a termination signal to the APM process
func (c *APMCheck) Stop() {
	if atomic.LoadUint32(&c.running) == 0 {
		log.Info("APM Agent not running.")
		return
	}

	c.stop <- struct{}{}
	<-c.stopDone
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
		return &APMCheck{
			stop:     make(chan struct{}),
			stopDone: make(chan struct{}),
		}
	}
	core.RegisterCheck("apm", factory)
}
