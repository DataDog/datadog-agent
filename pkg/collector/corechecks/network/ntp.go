// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package network

import (
	"expvar"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/beevik/ntp"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	cfg            *ntpConfig
	lastCollection time.Time
}

type ntpInstanceConfig struct {
	OffsetThreshold       int      `yaml:"offset_threshold"`
	Host                  string   `yaml:"host"`
	Hosts                 []string `yaml:"hosts"`
	Port                  string   `yaml:"port"`
	Timeout               int      `yaml:"timeout"`
	Version               int      `yaml:"version"`
	MinCollectionInterval int      `yaml:"min_collection_interval"`
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
	defaultMinCollectionInterval := 900 // 15 minutes, to follow pool.ntp.org's guidelines on the query rate
	defaultHosts := []string{"0.datadog.pool.ntp.org", "1.datadog.pool.ntp.org", "2.datadog.pool.ntp.org", "3.datadog.pool.ntp.org"}

	if err := yaml.Unmarshal(data, &instance); err != nil {
		return err
	}

	if err := yaml.Unmarshal(initData, &initConf); err != nil {
		return err
	}

	c.instance = instance
	if c.instance.Host != "" {
		hosts := []string{c.instance.Host}
		// If config contains both host and hosts
		for _, h := range c.instance.Hosts {
			if h != c.instance.Host {
				hosts = append(hosts, h)
			}
		}
		c.instance.Hosts = hosts
	}
	if c.instance.Hosts == nil {
		c.instance.Hosts = defaultHosts
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
	if c.instance.MinCollectionInterval == 0 {
		c.instance.MinCollectionInterval = defaultMinCollectionInterval
	}
	c.initConf = initConf

	return nil
}

// Configure configure the data from the yaml
func (c *NTPCheck) Configure(data integration.Data, initConfig integration.Data) error {
	err := c.CommonConfigure(data)
	if err != nil {
		return err
	}
	cfg := new(ntpConfig)
	err = cfg.parse(data, initConfig)
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
	if time.Now().Before(c.lastCollection.Add(time.Second * time.Duration(c.cfg.instance.MinCollectionInterval))) {
		log.Debugf("Skipping this check run, last run was less than %vs ago", c.cfg.instance.MinCollectionInterval)
		return nil
	}

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	var serviceCheckStatus metrics.ServiceCheckStatus
	serviceCheckMessage := ""
	offsetThreshold := c.cfg.instance.OffsetThreshold

	clockOffset, err := c.queryOffset()
	if err != nil {
		log.Info(err)
		serviceCheckStatus = metrics.ServiceCheckUnknown
	} else {
		if int(math.Abs(clockOffset)) > offsetThreshold {
			serviceCheckStatus = metrics.ServiceCheckCritical
			serviceCheckMessage = fmt.Sprintf("Offset %v is higher than offset threshold (%v secs)", clockOffset, offsetThreshold)
		} else {
			serviceCheckStatus = metrics.ServiceCheckOK
		}

		sender.Gauge("ntp.offset", clockOffset, "", nil)
		ntpExpVar.Set(clockOffset)
	}

	sender.ServiceCheck("ntp.in_sync", serviceCheckStatus, "", nil, serviceCheckMessage)

	c.lastCollection = time.Now()

	sender.Commit()

	return nil
}

func (c *NTPCheck) queryOffset() (float64, error) {
	offsets := []float64{}

	for _, host := range c.cfg.instance.Hosts {
		response, err := ntpQuery(host, c.cfg.instance.Version)
		if err != nil {
			log.Infof("There was an error querying the ntp host %s: %s", host, err)
		} else {
			offsets = append(offsets, response.ClockOffset.Seconds())
		}
	}

	if len(offsets) == 0 {
		return .0, fmt.Errorf("Failed to get clock offset from any ntp host")
	}

	var median float64

	sort.Float64s(offsets)
	length := len(offsets)
	if length%2 == 0 {
		median = (offsets[length/2-1] + offsets[length/2]) / 2.0
	} else {
		median = offsets[length/2]
	}

	return median, nil
}

func ntpFactory() check.Check {
	return &NTPCheck{
		CheckBase: core.NewCheckBase(ntpCheckName),
	}
}

func init() {
	core.RegisterCheck(ntpCheckName, ntpFactory)
}
