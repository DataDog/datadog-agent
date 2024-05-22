// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicediscovery contains the Service Discovery corecheck.
package servicediscovery

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=servicediscovery_mock.go

type osImpl interface {
	DiscoverServices() error
}

var (
	newOSImpl func(sender *telemetrySender, ignoreCfg map[string]bool) (osImpl, error)
)

const (
	// CheckName is the name of the check.
	CheckName = "service_discovery"

	refreshInterval = 1 * time.Minute
	heartbeatTime   = 15 * time.Minute
)

type config struct {
	IgnoreProcesses []string `yaml:"ignore_processes"`
}

type procStat struct {
	StartTime uint64
}

type serviceInfo struct {
	process       processInfo
	meta          serviceMetadata
	LastHeartbeat time.Time
}

type processInfo struct {
	PID     int
	CmdLine []string
	Env     []string
	Cwd     string
	Stat    procStat
	Ports   []int
}

// Parse parses the configuration
func (c *config) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	return nil
}

// Check reports discovered services.
type Check struct {
	corechecks.CheckBase
	cfg *config
	os  osImpl
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	// Since service_discovery is enabled by default, we want to prevent returning an error in Configure() for platforms
	// where the check is not implemented. Instead of that, we return an empty check.
	if newOSImpl == nil {
		return optional.NewNoneOption[func() check.Check]()
	}
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: corechecks.NewCheckBase(CheckName),
		cfg:       &config{},
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, instanceConfig, initConfig integration.Data, source string) error {
	if !pkgconfig.Datadog.GetBool("service_discovery.enabled") {
		return errors.New("service discovery is disabled")
	}
	if newOSImpl == nil {
		return errors.New("service_discovery check not implemented on " + runtime.GOOS)
	}
	if err := c.CommonConfigure(senderManager, initConfig, instanceConfig, source); err != nil {
		return err
	}
	if err := c.cfg.Parse(instanceConfig); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	ignoreCfg := map[string]bool{}
	for _, pName := range c.cfg.IgnoreProcesses {
		ignoreCfg[pName] = true
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}

	c.os, err = newOSImpl(newTelemetrySender(s), ignoreCfg)
	if err != nil {
		return err
	}

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

type timer interface {
	Now() time.Time
}

type realTime struct{}

func (realTime) Now() time.Time { return time.Now() }
