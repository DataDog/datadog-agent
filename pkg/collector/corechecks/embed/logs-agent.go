// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build log

package embed

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	log "github.com/cihub/seelog"
)

// LogsCheck keeps track of the running command
type LogsCheck struct {
	running  uint32
	stop     chan struct{}
	stopDone chan struct{}
}

func (c *LogsCheck) String() string {
	return "Logs Agent"
}

// Run executes the check with retries
func (c *LogsCheck) Run() error {
	atomic.StoreUint32(&c.running, 1)
	// TODO: retries should be configurable with meaningful default values
	err := check.Retry(defaultRetryDuration, defaultRetries, c.run, c.String())
	atomic.StoreUint32(&c.running, 0)

	return err
}

// run executes the check
func (c *LogsCheck) run() error {
	select {
	// poll the stop channel once to make sure no stop was requested since the last call to `run`
	case <-c.stop:
		log.Info("Not starting Logs check: stop requested")
		c.stopDone <- struct{}{}
		return nil
	default:
	}

	here, _ := executable.Folder()
	bin := path.Join(here, "logs-agent")

	cmd := exec.Command(
		bin,
		fmt.Sprintf("-ddconfig=%s", config.Datadog.ConfigFileUsed()),
		fmt.Sprintf("-ddconfd=%s", config.Datadog.GetString("confd_path")),
	)

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
			log.Errorf("unable to stop Logs check: %s", err)
		}
	}

	// wait for process to exit
	err = <-processDone
	c.stopDone <- struct{}{}
	return err
}

// Configure the LogsCheck. nothing to do
func (c *LogsCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	return nil
}

// InitSender initializes a sender but we don't need any
func (c *LogsCheck) InitSender() {}

// Interval returns the scheduling time for the check, this will be scheduled only once
// since `Run` won't return, thus implementing a long running check.
func (c *LogsCheck) Interval() time.Duration {
	return 0
}

// ID returns the name of the check since there should be only one instance running
func (c *LogsCheck) ID() check.ID {
	return "LOGS_AGENT"
}

// Stop sends a termination signal to the Logs process
func (c *LogsCheck) Stop() {
	if atomic.LoadUint32(&c.running) == 0 {
		log.Info("Logs Agent not running.")
		return
	}

	c.stop <- struct{}{}
	<-c.stopDone
}

// [TODO] The troubleshoot command does nothing for the Logs check
func (c *LogsCheck) Troubleshoot() error {
	return nil
}

func init() {
	factory := func() check.Check {
		return &LogsCheck{
			stop:     make(chan struct{}),
			stopDone: make(chan struct{}),
		}
	}
	core.RegisterCheck("logs-agent", factory)
}

// GetWarnings does not return anything
func (c *LogsCheck) GetWarnings() []error {
	return []error{}
}

// GetMetricStats returns the stats from the last run of the check, but there aren't any yet
func (c *LogsCheck) GetMetricStats() (map[string]int64, error) {
	return make(map[string]int64), nil
}
