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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=servicediscovery_mock.go

const (
	// CheckName is the name of the check.
	CheckName = "service_discovery"

	refreshInterval = 1 * time.Minute
	heartbeatTime   = 15 * time.Minute
)

type serviceInfo struct {
	meta          ServiceMetadata
	service       model.Service
	LastHeartbeat time.Time
}

type serviceEvents struct {
	start     []serviceInfo
	stop      []serviceInfo
	heartbeat []serviceInfo
}

type discoveredServices struct {
	ignoreProcs     map[int]bool
	potentials      map[int]*serviceInfo
	runningServices map[int]*serviceInfo

	events serviceEvents
}

type osImpl interface {
	DiscoverServices() (*discoveredServices, error)
}

var newOSImpl func(ignoreCfg map[string]bool, containerProvider proccontainers.ContainerProvider) (osImpl, error)

type config struct {
	IgnoreProcesses []string `yaml:"ignore_processes"`
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
	cfg                   *config
	os                    osImpl
	sender                *telemetrySender
	sentRepeatedEventPIDs map[int]bool
	containerProvider     proccontainers.ContainerProvider
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	// Since service_discovery is enabled by default, we want to prevent returning an error in Configure() for platforms
	// where the check is not implemented. Instead of that, we return an empty check.
	if newOSImpl == nil {
		return optional.NewNoneOption[func() check.Check]()
	}

	sharedContainerProvider, err := proccontainers.GetSharedContainerProvider()

	if err != nil {
		return optional.NewNoneOption[func() check.Check]()
	}

	return optional.NewOption(func() check.Check {
		return newCheck(sharedContainerProvider)
	})
}

// TODO: add metastore param
func newCheck(containerProvider proccontainers.ContainerProvider) *Check {
	return &Check{
		CheckBase:             corechecks.NewCheckBase(CheckName),
		cfg:                   &config{},
		sentRepeatedEventPIDs: make(map[int]bool),
		containerProvider:     containerProvider,
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, instanceConfig, initConfig integration.Data, source string) error {
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
	c.sender = newTelemetrySender(s)

	c.os, err = newOSImpl(ignoreCfg, c.containerProvider)
	if err != nil {
		return err
	}

	return nil
}

// Run executes the check.
func (c *Check) Run() error {
	if !pkgconfigsetup.SystemProbe().GetBool("discovery.enabled") {
		return nil
	}

	start := time.Now()
	defer func() {
		diff := time.Since(start).Seconds()
		metricTimeToScan.Observe(diff)
	}()

	disc, err := c.os.DiscoverServices()
	if err != nil {
		telemetryFromError(err)
		return err
	}

	log.Debugf("ignoreProcs: %d | runningServices: %d | potentials: %d",
		len(disc.ignoreProcs),
		len(disc.runningServices),
		len(disc.potentials),
	)
	metricDiscoveredServices.Set(float64(len(disc.runningServices)))

	runningServicesByName := map[string][]*serviceInfo{}
	for _, svc := range disc.runningServices {
		runningServicesByName[svc.meta.Name] = append(runningServicesByName[svc.meta.Name], svc)
	}
	for _, svcs := range runningServicesByName {
		if len(svcs) <= 1 {
			continue
		}
		for _, svc := range svcs {
			if c.sentRepeatedEventPIDs[svc.service.PID] {
				continue
			}
			err := fmt.Errorf("found repeated service name: %s", svc.meta.Name)
			telemetryFromError(errWithCode{
				err:  err,
				code: errorCodeRepeatedServiceName,
				svc:  &svc.meta,
			})
			// track the PID, so we don't increase this counter in every run of the check.
			c.sentRepeatedEventPIDs[svc.service.PID] = true
		}
	}

	potentialNames := map[string]bool{}
	for _, p := range disc.potentials {
		potentialNames[p.meta.Name] = true
	}

	// group events by name in order to find repeated events for the same service name.
	eventsByName := make(eventsByNameMap)
	for _, p := range disc.events.start {
		eventsByName.addStart(p)
	}
	for _, p := range disc.events.heartbeat {
		eventsByName.addHeartbeat(p)
	}
	for _, p := range disc.events.stop {
		if potentialNames[p.meta.Name] {
			// we consider this situation a restart, so we skip the stop event.
			log.Debugf("there is a potential service with the same name as a stopped one, skipping end-service event (name: %q)", p.meta.Name)
			continue
		}
		eventsByName.addStop(p)
		if c.sentRepeatedEventPIDs[p.service.PID] {
			// delete this process from the map, so we track it if the PID gets reused
			delete(c.sentRepeatedEventPIDs, p.service.PID)
		}
	}

	for name, ev := range eventsByName {
		if len(ev.start) > 0 && len(ev.stop) > 0 || len(ev.heartbeat) > 0 && len(ev.stop) > 0 {
			// this is a consequence of the possibility of generating the same service name for different processes.
			// at this point, we just skip the end-service events so at least these services don't disappear in the UI.
			log.Debugf("got multiple start/heartbeat/end service events for the same service name, skipping end-service events (name: %q)", name)
			clear(ev.stop)
		}
		for _, svc := range ev.start {
			c.sender.sendStartServiceEvent(svc)
		}
		for _, svc := range ev.heartbeat {
			c.sender.sendHeartbeatServiceEvent(svc)
		}
		for _, svc := range ev.stop {
			c.sender.sendEndServiceEvent(svc)
		}
	}

	return nil
}

type eventsByNameMap map[string]*serviceEvents

func (m eventsByNameMap) addStart(svc serviceInfo) {
	events, ok := m[svc.meta.Name]
	if !ok {
		events = &serviceEvents{}
	}
	events.start = append(events.start, svc)
	m[svc.meta.Name] = events
}

func (m eventsByNameMap) addHeartbeat(svc serviceInfo) {
	events, ok := m[svc.meta.Name]
	if !ok {
		events = &serviceEvents{}
	}
	events.heartbeat = append(events.heartbeat, svc)
	m[svc.meta.Name] = events
}

func (m eventsByNameMap) addStop(svc serviceInfo) {
	events, ok := m[svc.meta.Name]
	if !ok {
		events = &serviceEvents{}
	}
	events.stop = append(events.stop, svc)
	m[svc.meta.Name] = events
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
