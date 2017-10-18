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
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/kardianos/osext"
)

// LogsCheck keeps track of the running command
type LogsCheck struct {
	cmd *exec.Cmd
}

func (c *LogsCheck) String() string {
	return "Logs Agent"
}

// Run executes the check
func (c *LogsCheck) Run() error {
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

// Configure the LogsCheck
func (c *LogsCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	here, _ := osext.ExecutableFolder()
	confd := config.Datadog.GetString("confd_path")
	ddcondig := filepath.Clean(filepath.Join(confd, ".."))
	bin := path.Join(here, "logs-agent")

	c.cmd = exec.Command(
		bin,
		fmt.Sprintf("-ddconfig=%s", ddcondig),
	)

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
	err := c.cmd.Process.Signal(os.Kill)
	if err != nil {
		log.Errorf("unable to stop Logs check: %s", err)
	}
}

func init() {
	factory := func() check.Check {
		return &LogsCheck{}
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
