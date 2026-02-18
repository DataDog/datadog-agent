// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ntp implements the ntp check.
package ntp

import (
	"context"
	"expvar"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/beevik/ntp"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName                    = "ntp"
	defaultMinCollectionInterval = 900 // 15 minutes, to follow pool.ntp.org's guidelines on the query rate
)

var (
	ntpExpVar = expvar.NewFloat("ntpOffset")
	// for testing purpose
	ntpQuery = ntp.QueryWithOptions

	tlmNtpOffset = telemetry.NewGauge("check", "ntp_offset",
		nil, "Ntp offset")

	defaultDatadogPool = []string{"0.datadog.pool.ntp.org", "1.datadog.pool.ntp.org", "2.datadog.pool.ntp.org", "3.datadog.pool.ntp.org"}
)

// NTPCheck only has sender and config
type NTPCheck struct {
	core.CheckBase
	cfg            *ntpConfig
	lastCollection time.Time
}

type ntpInstanceConfig struct {
	OffsetThreshold        int      `yaml:"offset_threshold"`
	Host                   string   `yaml:"host"`
	Hosts                  []string `yaml:"hosts"`
	Port                   int      `yaml:"port"`
	Timeout                int      `yaml:"timeout"`
	Version                int      `yaml:"version"`
	UseLocalDefinedServers bool     `yaml:"use_local_defined_servers"`
}

type ntpInitConfig struct{}

type ntpConfig struct {
	instance ntpInstanceConfig
	initConf ntpInitConfig
}

func (c *NTPCheck) String() string {
	return "ntp"
}

// for testing
var getCloudProviderNTPHosts = cloudproviders.GetCloudProviderNTPHosts
var getLocalDefinedNTPServersFunc = getLocalDefinedNTPServers

// stringSlicesEqual compares two string slices for equality
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func (c *ntpConfig) parse(data []byte, initData []byte, getLocalServers func() ([]string, error)) error {
	var instance ntpInstanceConfig
	var initConf ntpInitConfig
	defaultVersion := 3
	defaultTimeout := 5
	defaultPort := 123
	defaultOffsetThreshold := 60

	defaultHosts := getCloudProviderNTPHosts(context.TODO())

	// Default to our domains on pool.ntp.org if no cloud provider detected
	if defaultHosts == nil {
		log.Debugf("No cloud provider detected, using default ntp pool: [ %s ]", strings.Join(defaultDatadogPool, ", "))
		defaultHosts = defaultDatadogPool
	}

	if err := yaml.Unmarshal(data, &instance); err != nil {
		return err
	}

	if err := yaml.Unmarshal(initData, &initConf); err != nil {
		return err
	}

	c.instance = instance
	var localNtpServers []string
	var err error
	if c.instance.UseLocalDefinedServers {
		localNtpServers, err = getLocalServers()
		if err != nil {
			log.Warnf("Could not get local NTP servers, falling back to configured hosts: %v", err)
		} else {
			log.Debugf("Detected local defined servers: [ %s ]", strings.Join(localNtpServers, ", "))
		}
	}

	if len(localNtpServers) > 0 {
		c.instance.Hosts = localNtpServers
	} else if c.instance.Host != "" {
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

	log.Infof("Using NTP servers: [ %s ]", strings.Join(c.instance.Hosts, ", "))

	if c.instance.Port == 0 {
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
func (c *NTPCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	cfg := new(ntpConfig)
	err := cfg.parse(data, initConfig, getLocalDefinedNTPServersFunc)
	if err != nil {
		log.Errorf("Error parsing configuration file: %s", err)
		return err
	}

	c.BuildID(integrationConfigDigest, data, initConfig)
	c.cfg = cfg

	err = c.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the check
func (c *NTPCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	// Re-discover NTP servers on every run for dynamic environments (e.g., DC promotions)
	if c.cfg.instance.UseLocalDefinedServers {
		servers, err := getLocalDefinedNTPServersFunc()
		if err != nil {
			log.Warnf("Could not re-discover NTP servers: %v", err)
		} else if len(servers) > 0 {
			// Check if servers have changed
			// Sort slices for order-insensitive comparison
			sort.Strings(c.cfg.instance.Hosts)
			sort.Strings(servers)
			if !stringSlicesEqual(c.cfg.instance.Hosts, servers) {
				log.Infof("NTP servers changed from [%s] to [%s]",
					strings.Join(c.cfg.instance.Hosts, ", "),
					strings.Join(servers, ", "))
				c.cfg.instance.Hosts = servers // Update the list of hosts for this run
			}
			// Silent when no change - removed repetitive debug logging
		}
	}

	var serviceCheckStatus servicecheck.ServiceCheckStatus
	serviceCheckMessage := ""
	offsetThreshold := c.cfg.instance.OffsetThreshold

	// Submit intake offset first (captured from forwarder responses)
	// This is independent of NTP check success
	if intakeOffsetVar := expvar.Get("corechecks_net_ntp_intake_time_offset"); intakeOffsetVar != nil {
		if floatVar, ok := intakeOffsetVar.(*expvar.Float); ok {
			intakeOffset := floatVar.Value()
			if !math.IsNaN(intakeOffset) {
				// Calculate what the intake server's time would be by applying the offset to current time
				// Using intake server's time as the metric timestamp ensures it appears correctly
				// in Datadog even when the agent's clock is drifted
				// (positive offset = agent behind, negative = agent ahead)
				currentTime := time.Now()
				intakeServerTime := currentTime.Add(time.Duration(intakeOffset * float64(time.Second)))
				intakeTS := float64(intakeServerTime.UnixNano()) / 1e9
				_ = sender.GaugeWithTimestamp("ntp.offset", intakeOffset, "", []string{"source:intake"}, intakeTS)
			}
		}
	}

	clockOffset, ts, err := c.queryOffset()
	if err != nil {
		log.Error(err)

		sender.ServiceCheck("ntp.in_sync", servicecheck.ServiceCheckUnknown, "", nil, serviceCheckMessage)
		c.lastCollection = time.Now()
		sender.Commit()

		return err
	}
	if int(math.Abs(clockOffset)) > offsetThreshold {
		serviceCheckStatus = servicecheck.ServiceCheckCritical
		serviceCheckMessage = fmt.Sprintf("Offset %v is higher than offset threshold (%v secs)", clockOffset, offsetThreshold)
	} else {
		serviceCheckStatus = servicecheck.ServiceCheckOK
	}

	_ = sender.GaugeWithTimestamp("ntp.offset", clockOffset, "", []string{"source:ntp"}, ts)
	ntpExpVar.Set(clockOffset)
	tlmNtpOffset.Set(clockOffset)

	sender.ServiceCheck("ntp.in_sync", serviceCheckStatus, "", nil, serviceCheckMessage)

	c.lastCollection = time.Now()

	sender.Commit()

	return nil
}

func (c *NTPCheck) queryOffset() (float64, float64, error) {
	type sample struct {
		offset    float64
		timestamp float64
	}

	samples := []sample{}

	for _, host := range c.cfg.instance.Hosts {
		response, err := ntpQuery(host, ntp.QueryOptions{
			Version: c.cfg.instance.Version,
			Port:    c.cfg.instance.Port,
			Timeout: time.Duration(c.cfg.instance.Timeout) * time.Second,
		})
		if err != nil {
			log.Debugf("Error querying ntp host %s: %s", host, err)
			continue
		}

		if err := response.Validate(); err != nil {
			log.Infof("Invalid ntp response for host %s: %s", host, err)
			continue
		}

		samples = append(samples, sample{
			offset:    response.ClockOffset.Seconds(),
			timestamp: float64(response.Time.UnixNano()) / 1e9, // fractional seconds since epoch
		})
	}

	if len(samples) == 0 {
		return 0, 0, fmt.Errorf(
			"failed to get clock offset from any ntp host: [ %s ]. See https://docs.datadoghq.com/agent/troubleshooting/ntp/ for more details on how to debug this issue",
			strings.Join(c.cfg.instance.Hosts, ", "),
		)
	}

	// ---- choose timestamp from sample with absolute offset closest to zero ----
	bestTS := samples[0].timestamp
	bestAbs := math.Abs(samples[0].offset)

	for _, s := range samples[1:] {
		if abs := math.Abs(s.offset); abs < bestAbs {
			bestAbs = abs
			bestTS = s.timestamp
		}
	}

	// ---- compute median offset (existing behavior) ----
	offsets := make([]float64, len(samples))
	for i, s := range samples {
		offsets[i] = s.offset
	}

	sort.Float64s(offsets)
	mid := len(offsets) / 2

	var median float64
	if len(offsets)%2 == 1 {
		median = offsets[mid]
	} else {
		median = (offsets[mid-1] + offsets[mid]) / 2
	}

	return median, bestTS, nil
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &NTPCheck{
		CheckBase: core.NewCheckBaseWithInterval(CheckName, time.Duration(defaultMinCollectionInterval)*time.Second),
	}
}
