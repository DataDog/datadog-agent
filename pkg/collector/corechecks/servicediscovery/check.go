// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/procfs"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check.
	CheckName = "service_discovery"
	// refreshInterval = 1 * time.Minute
	// heartbeatTime   = 15 * time.Minute

	refreshInterval = 10 * time.Second
	heartbeatTime   = 1 * time.Minute
)

// Config holds the check configuration.
type config struct {
	IgnoreProcesses string `yaml:"ignore_processes"`
}

type processInfo struct {
	PID       int
	Name      string
	ShortName string
	CmdLine   []string
	Env       []string
	Cwd       string

	// TODO: change this to not use the procfs struct
	Stat             *procfs.ProcStat
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

type aliveServices struct {
	m  map[int]*processInfo
	mu sync.RWMutex
}

func (a *aliveServices) get(pid int) (*processInfo, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	v, ok := a.m[pid]
	return v, ok
}

func (a *aliveServices) set(pid int, info *processInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m[pid] = info
}

func (a *aliveServices) delete(pid int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.m, pid)
}

type ignoreProcesses struct {
	m  map[int]string
	mu sync.RWMutex
}

func (i *ignoreProcesses) get(pid int) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	v, ok := i.m[pid]
	return v, ok
}

func (i *ignoreProcesses) set(pid int, name string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.m[pid] = name
}

func (i *ignoreProcesses) delete(pid int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.m, pid)
}

type Check struct {
	corechecks.CheckBase
	cfg    *config
	sender sender.Sender

	// set of process names that should always be ignored
	alwaysIgnore map[string]struct{}

	// PID -> process name
	// pids will be added here because the name was ignored in the config, or because
	// it was already scanned and had no open ports.
	ignore   *ignoreProcesses
	services *aliveServices
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase:    corechecks.NewCheckBase(CheckName),
		cfg:          &config{},
		alwaysIgnore: make(map[string]struct{}),
		ignore: &ignoreProcesses{
			m: make(map[int]string),
		},
		services: &aliveServices{
			m: make(map[int]*processInfo),
		},
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	if !pkgconfig.Datadog.GetBool("service_discovery.enabled") {
		return errors.New("service discovery is disabled")
	}
	if err := c.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source); err != nil {
		return err
	}
	if err := c.cfg.Parse(data); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	for _, pName := range strings.Split(c.cfg.IgnoreProcesses, ",") {
		c.alwaysIgnore[pName] = struct{}{}
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	c.sender = s

	return nil
}

// Run executes the check.
func (c *Check) Run() error {
	return c.discoverServices()
}

// Interval returns how often the check should run.
func (c *Check) Interval() time.Duration {
	return refreshInterval
}
