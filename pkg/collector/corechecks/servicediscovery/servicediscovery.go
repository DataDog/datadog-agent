// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicediscovery contains the Service Discovery corecheck.
package servicediscovery

import (
	"errors"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=servicediscovery_mock.go

const (
	// CheckName is the name of the check.
	CheckName = "service_discovery"

	refreshInterval = 1 * time.Minute
)

type osImpl interface {
	DiscoverServices() (*model.ServicesResponse, error)
}

var newOSImpl func() (osImpl, error)

// Check reports discovered services.
type Check struct {
	corechecks.CheckBase
	os     osImpl
	sender *telemetrySender
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	// Since service_discovery is enabled by default, we want to prevent returning an error in Configure() for platforms
	// where the check is not implemented. Instead of that, we return an empty check.
	if newOSImpl == nil {
		return option.None[func() check.Check]()
	}

	return option.New(func() check.Check {
		return newCheck()
	})
}

// TODO: add metastore param
func newCheck() *Check {
	return &Check{
		CheckBase: corechecks.NewCheckBase(CheckName),
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, instanceConfig, initConfig integration.Data, source string) error {
	if newOSImpl == nil {
		return errors.New("service_discovery check not implemented on " + runtime.GOOS)
	}
	if !pkgconfigsetup.SystemProbe().GetBool("discovery.enabled") {
		return errors.New("service discovery is disabled")
	}
	if err := c.CommonConfigure(senderManager, initConfig, instanceConfig, source); err != nil {
		return err
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	c.sender = newTelemetrySender(s)

	c.os, err = newOSImpl()
	if err != nil {
		return err
	}

	return nil
}

// Run executes the check.
func (c *Check) Run() error {
	response, err := c.os.DiscoverServices()
	if err != nil {
		return err
	}

	log.Debugf("runningServices: %d", response.RunningServicesCount)
	metricDiscoveredServices.Set(float64(response.RunningServicesCount))

	for _, p := range response.StartedServices {
		c.sender.sendStartServiceEvent(p)
	}
	for _, p := range response.HeartbeatServices {
		c.sender.sendHeartbeatServiceEvent(p)
	}
	for _, p := range response.StoppedServices {
		c.sender.sendEndServiceEvent(p)
	}

	return nil
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
