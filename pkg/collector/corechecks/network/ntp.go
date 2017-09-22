// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package network

import (
	"expvar"
	"fmt"
	"math/rand"
	"time"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/beevik/ntp"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	yaml "gopkg.in/yaml.v2"
)

var ntpExpVar = expvar.NewFloat("ntpOffset")

// NTPCheck only has sender and config
type NTPCheck struct {
	id           check.ID
	lastWarnings []error
	cfg          *ntpConfig
}

type ntpInstanceConfig struct {
	OffsetThreshold int    `yaml:"offset_threshold"`
	Host            string `yaml:"host"`
	Port            string `yaml:"port"`
	Timeout         int    `yaml:"timeout"`
	Version         int    `yaml:"version"`
}

type ntpInitConfig struct{}

type ntpConfig struct {
	instance ntpInstanceConfig
	initConf ntpInitConfig
}

func (c *NTPCheck) String() string {
	return "ntp"
}

func (c *ntpConfig) Parse(data []byte, initData []byte) error {
	var instance ntpInstanceConfig
	var initConf ntpInitConfig
	defaultVersion := 3
	defaultTimeout := 1
	defaultPort := "ntp"
	defaultOffsetThreshold := 60

	if err := yaml.Unmarshal(data, &instance); err != nil {
		return err
	}

	if err := yaml.Unmarshal(initData, &initConf); err != nil {
		return err
	}

	c.instance = instance
	if c.instance.Host == "" {
		c.instance.Host = fmt.Sprintf("%d.datadog.pool.ntp.org", rand.Intn(3))
	}
	if c.instance.Port == "" {
		c.instance.Port = defaultPort
	}
	if c.instance.Version == 0 {
		c.instance.Version = defaultVersion
	}
	if c.instance.Timeout == 0 {
		c.instance.Timeout = defaultTimeout
	}
	if c.instance.OffsetThreshold == 0 {
		c.instance.OffsetThreshold = defaultOffsetThreshold
	}
	c.initConf = initConf

	return nil
}

// Configure configure the data from the yaml
func (c *NTPCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	cfg := new(ntpConfig)
	err := cfg.Parse(data, initConfig)
	if err != nil {
		log.Criticalf("Error parsing configuration file: %s", err)
		return err
	}

	c.id = check.Identify(c, data, initConfig)
	c.cfg = cfg

	return nil
}

// ID returns the id of the instance
func (c *NTPCheck) ID() check.ID {
	return c.id
}

// Interval returns the scheduling time for the check
func (c *NTPCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// Stop does nothing
func (c *NTPCheck) Stop() {}

// Run runs the check
func (c *NTPCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	var serviceCheckStatus metrics.ServiceCheckStatus
	var clockOffset int
	serviceCheckMessage := ""
	offsetThreshold := c.cfg.instance.OffsetThreshold

	response, err := ntp.Query(c.cfg.instance.Host, c.cfg.instance.Version)
	if err != nil {
		log.Infof("There was an error querying the ntp host: %s", err)
		serviceCheckStatus = metrics.ServiceCheckUnknown
	} else {
		clockOffset = int(response.ClockOffset.Seconds())
		if clockOffset > offsetThreshold {
			serviceCheckStatus = metrics.ServiceCheckCritical
			serviceCheckMessage = fmt.Sprintf("Offset %v secs higher than offset threshold (%v secs)", clockOffset, offsetThreshold)
		} else {
			serviceCheckStatus = metrics.ServiceCheckOK
		}

		sender.Gauge("ntp.offset", response.ClockOffset.Seconds(), "", nil)
		ntpExpVar.Set(response.ClockOffset.Seconds())
	}

	sender.ServiceCheck("ntp.in_sync", serviceCheckStatus, "", nil, serviceCheckMessage)

	sender.Commit()

	return nil
}

// GetWarnings grabs the last warnings from the sender
func (c *NTPCheck) GetWarnings() []error {
	w := c.lastWarnings
	c.lastWarnings = []error{}
	return w
}

// Warn will log a warning and add it to the warnings
func (c *NTPCheck) warn(v ...interface{}) error {
	w := log.Warn(v)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// Warnf will log a formatted warning and add it to the warnings
func (c *NTPCheck) warnf(format string, params ...interface{}) error {
	w := log.Warnf(format, params)
	c.lastWarnings = append(c.lastWarnings, w)

	return w
}

// GetMetricStats returns the stats from the last run of the check
func (c *NTPCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

func ntpFactory() check.Check {
	return &NTPCheck{}
}

func init() {
	core.RegisterCheck("ntp", ntpFactory)
}
