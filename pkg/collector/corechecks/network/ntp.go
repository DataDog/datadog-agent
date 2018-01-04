// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package network

import (
	"expvar"
	"fmt"
	"math/rand"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/beevik/ntp"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	yaml "gopkg.in/yaml.v2"
)

const ntpCheckName = "ntp"

var (
	ntpExpVar = expvar.NewFloat("ntpOffset")
	// for testing purpose
	ntpQuery = ntp.Query
)

// NTPCheck only has sender and config
type NTPCheck struct {
	core.CheckBase
	cfg *ntpConfig
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

func (c *ntpConfig) parse(data []byte, initData []byte) error {
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
	err := cfg.parse(data, initConfig)
	if err != nil {
		log.Criticalf("Error parsing configuration file: %s", err)
		return err
	}

	c.BuildID(data, initConfig)
	c.cfg = cfg

	return nil
}

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

	response, err := ntpQuery(c.cfg.instance.Host, c.cfg.instance.Version)
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

func ntpFactory() check.Check {
	return &NTPCheck{
		CheckBase: core.NewCheckBase(ntpCheckName),
	}
}

func init() {
	core.RegisterCheck(ntpCheckName, ntpFactory)
}
