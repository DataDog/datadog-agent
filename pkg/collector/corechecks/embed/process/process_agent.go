// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

//nolint:revive // TODO(PLINT) Fix revive linter
package process

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go.uber.org/atomic"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed/common"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "process_agent"
)

type processAgentCheckConf struct {
	BinPath string `yaml:"bin_path,omitempty"`
}

// ProcessAgentCheck keeps track of the running command
type ProcessAgentCheck struct {
	binPath        string
	commandOpts    []string
	running        *atomic.Bool
	stop           chan struct{}
	stopDone       chan struct{}
	source         string
	telemetry      bool
	initConfig     string
	instanceConfig string
}

// String displays the Agent name
func (c *ProcessAgentCheck) String() string {
	return "Process Agent"
}

// Version displays the command's version
func (c *ProcessAgentCheck) Version() string {
	return ""
}

// ConfigSource displays the command's source
func (c *ProcessAgentCheck) ConfigSource() string {
	return c.source
}

// Loader returns the check loader
func (*ProcessAgentCheck) Loader() string {
	// the process check is scheduled by the Go loader
	return corechecks.GoCheckLoaderName
}

// InitConfig returns the init configuration
func (c *ProcessAgentCheck) InitConfig() string {
	return c.initConfig
}

// InstanceConfig returns the instance configuration
func (c *ProcessAgentCheck) InstanceConfig() string {
	return c.instanceConfig
}

// Run executes the check with retries
func (c *ProcessAgentCheck) Run() error {
	c.running.Store(true)
	// TODO: retries should be configurable with meaningful default values
	err := check.Retry(common.DefaultRetryDuration, common.DefaultRetries, c.run, c.String())
	c.running.Store(false)

	return err
}

// run executes the check
func (c *ProcessAgentCheck) run() error {
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
		return common.RetryExitError(err)
	}

	processDone := make(chan error)
	go func() {
		processDone <- cmd.Wait()
	}()

	select {
	case err = <-processDone:
		return common.RetryExitError(err)
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
//
//nolint:revive // TODO(PROC) Fix revive linter
func (c *ProcessAgentCheck) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	// only log whether process check is enabled or not but don't return early, because we still need to initialize "binPath", "source" and
	// start up process-agent. Ultimately it's up to process-agent to decide whether to run or not based on the config
	if enabled := pkgconfigsetup.Datadog().GetBool("process_config.process_collection.enabled"); !enabled {
		log.Info("live process monitoring is disabled through main configuration file")
	}

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

	// be explicit about the config file location
	configFile := pkgconfigsetup.Datadog().ConfigFileUsed()
	c.commandOpts = []string{}
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		c.commandOpts = append(c.commandOpts, fmt.Sprintf("-config=%s", configFile))
	}

	c.source = source
	c.telemetry = utils.IsCheckTelemetryEnabled("process_agent", pkgconfigsetup.Datadog())
	c.initConfig = string(initConfig)
	c.instanceConfig = string(data)
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
func (c *ProcessAgentCheck) ID() checkid.ID {
	return "PROCESS_AGENT"
}

// IsTelemetryEnabled returns if the telemetry is enabled for this check
func (c *ProcessAgentCheck) IsTelemetryEnabled() bool {
	return c.telemetry
}

// Stop sends a termination signal to the process-agent process
func (c *ProcessAgentCheck) Stop() {
	if !c.running.Load() {
		log.Info("Process Agent not running.")
		return
	}

	c.stop <- struct{}{}
	<-c.stopDone
}

// Cancel does nothing
func (c *ProcessAgentCheck) Cancel() {}

// GetSenderStats returns the stats from the last run of the check, but there aren't any yet
func (c *ProcessAgentCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.NewSenderStats(), nil
}

// GetDiagnoses returns the diagnoses of the check
func (c *ProcessAgentCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return nil, nil
}

// IsHASupported returns if the check is compatible with High Availability
func (c *ProcessAgentCheck) IsHASupported() bool {
	return false
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &ProcessAgentCheck{
		stop:     make(chan struct{}),
		stopDone: make(chan struct{}),
		running:  atomic.NewBool(false),
	}
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
