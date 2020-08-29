// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package nvidia

import (
	"bufio"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const (
	tegraCheckName			= "tegra"
	defaultRetryDuration	= 5 * time.Second
	defaultRetries			= 3
)

func execTegraStats() (string, error) {
	return "", nil
}

// retryExitError converts `exec.ExitError`s to `check.RetryableError`s, so that checks using this
// are retried.
func retryExitError(err error) error { // nolint Used only on some architectures
	switch err.(type) {
	case *exec.ExitError: // error type returned when the process exits with non-zero status
		return check.RetryableError{Err: err}
	default:
		return err
	}
}

// TegraCheck contains the field for the TegraCheck
type TegraCheck struct {
	core.CheckBase

	// Indicates that this check has been scheduled and is running.
	running			uint32

	// The path to the tegrastats binary. Defaults to /usr/bin/tegrastats
	tegraStatsPath	string

	// The command line options for tegrastats
	commandOpts		[]string

	stop        chan struct{}
	stopDone    chan struct{}
}

// Interval returns the scheduling time for the check.
// Returns 0 since we're a long-running check.
func (c *TegraCheck) Interval() time.Duration {
	return 0
}

func sendRamMetrics(sender aggregator.Sender, field string) {

}

// Run executes the check
func (c *TegraCheck) Run() error {
	atomic.StoreUint32(&c.running, 1)
	err := check.Retry(defaultRetryDuration, defaultRetries, c.run, c.String())
	atomic.StoreUint32(&c.running, 0)

	return err
}

func (c *TegraCheck) processTegraStatsOutput(tegraStatsOuptut string) error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	fields := strings.Fields(tegraStatsOuptut)
	for _, field := range fields {
		if strings.ToLower(field) == "ram" {
			
		}
	}
	sender.Gauge("system.gpu.mem.used", 128, "", nil)
	sender.Gauge("system.gpu.mem.total", 1024, "", nil)

	// lfb NxXMB, X is the largest free block. N is the number of free blocks of this size.
	// NB: This is Nvidia specific
	sender.Gauge("system.gpu.mem.n_lfb", 4, "", nil)
	sender.Gauge("system.gpu.mem.lfb", 16, "", nil)

	// We skip the CPU stats returned by TegraStats because it is duplicated with the CPU check

	sender.Commit()
	return nil
}

func (c *TegraCheck) run() error {
	select {
	// poll the stop channel once to make sure no stop was requested since the last call to `run`
	case <-c.stop:
		log.Info("Not starting %s check: stop requested", tegraCheckName)
		c.stopDone <- struct{}{}
		return nil
	default:
	}

	cmd := exec.Command(c.tegraStatsPath, c.commandOpts...)

	// Parse the standard output for the stats
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			if err = c.processTegraStatsOutput(in.Text()); err != nil {
				_ = log.Error(err)
			}
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
			_ = log.Error(in.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
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
			_ = log.Errorf("unable to stop %s check: %s", tegraCheckName, err)
		}
	}

	err = <-processDone
	c.stopDone <- struct{}{}
	return err
}

// Configure the GPU check
func (c *TegraCheck) Configure(data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(data, source)
	if err != nil {
		return err
	}

	// TODO: Make this configurable
	c.tegraStatsPath = "/usr/bin/tegrastats"

	// Since our interval is 0 because we're a long running check, we can use the CheckBase.Interval() as
	// the tegrastats reporting interval
	c.commandOpts = []string{
		fmt.Sprintf("--interval %d", int64(c.CheckBase.Interval() * time.Millisecond)),
	}

	return nil
}

func tegraCheckFactory() check.Check {
	return &TegraCheck{
		CheckBase: core.NewCheckBase(tegraCheckName),
	}
}

func init() {
	core.RegisterCheck(tegraCheckName, tegraCheckFactory)
}
