// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type osImpl interface {
	DiscoverServices() error
}

var (
	newOSImpl func(sender *telemetrySender, ignoreCfg map[string]struct{}) osImpl
)

const (
	// CheckName is the name of the check.
	CheckName = "service_discovery"
	// TODO: use these values before merging
	// refreshInterval = 1 * time.Minute
	// heartbeatTime   = 15 * time.Minute

	refreshInterval = 10 * time.Second
	heartbeatTime   = 1 * time.Minute
)

// Config holds the check configuration.
type config struct {
	IgnoreProcesses string `yaml:"ignore_processes"`
}

type procStat struct {
	StartTime uint64
}

type processInfo struct {
	PID              int
	Name             string
	ShortName        string
	CmdLine          []string
	Env              []string
	Cwd              string
	Stat             *procStat
	Ports            []int
	DetectedTime     time.Time
	LastHeatBeatTime time.Time
}

// Parse parses the configuration
func (c *config) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	return nil
}

type Check struct {
	corechecks.CheckBase
	cfg *config
	os  osImpl
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: corechecks.NewCheckBase(CheckName),
		cfg:       &config{},
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	if !pkgconfig.Datadog.GetBool("service_discovery.enabled") {
		return errors.New("service discovery is disabled")
	}

	if newOSImpl == nil {
		return errors.New("service_discovery check not implemented on " + runtime.GOOS)
	}

	if err := c.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source); err != nil {
		return err
	}

	if err := c.cfg.Parse(data); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	ignoreCfg := map[string]struct{}{}
	for _, pName := range strings.Split(c.cfg.IgnoreProcesses, ",") {
		ignoreCfg[pName] = struct{}{}
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}

	c.os = newOSImpl(newTelemetrySender(s), ignoreCfg)

	return nil
}

// Run executes the check.
func (c *Check) Run() error {
	return c.os.DiscoverServices()
}

// Interval returns how often the check should run.
func (c *Check) Interval() time.Duration {
	return refreshInterval
}
