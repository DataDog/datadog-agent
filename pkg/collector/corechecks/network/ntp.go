package network

import (
	"fmt"
	"math/rand"
	"time"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/beevik/ntp"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	yaml "gopkg.in/yaml.v2"
)

// NTPCheck only has sender and config
type NTPCheck struct {
	id     check.ID
	sender aggregator.Sender
	cfg    *ntpConfig
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
	return "NTPCheck"
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

// InitSender initializes a sender
func (c *NTPCheck) InitSender() {
	s, err := aggregator.GetSender(c.ID())
	if err != nil {
		log.Error(err)
		return
	}

	c.sender = s
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
	var serviceCheckStatus aggregator.ServiceCheckStatus
	var clockOffset int
	serviceCheckMessage := ""
	offsetThreshold := c.cfg.instance.OffsetThreshold

	response, err := ntp.Query(c.cfg.instance.Host, c.cfg.instance.Version)
	if err != nil {
		log.Infof("There was an error querying the ntp host: %s", err)
		serviceCheckStatus = aggregator.ServiceCheckUnknown
	} else {
		clockOffset = int(response.ClockOffset.Seconds())
		if clockOffset > offsetThreshold {
			serviceCheckStatus = aggregator.ServiceCheckCritical
			serviceCheckMessage = fmt.Sprintf("Offset %v secs higher than offset threshold (%v secs)", clockOffset, offsetThreshold)
		} else {
			serviceCheckStatus = aggregator.ServiceCheckOK
		}

		c.sender.Gauge("ntp.offset", response.ClockOffset.Seconds(), "", nil)
	}

	c.sender.ServiceCheck("ntp.in_sync", serviceCheckStatus, "", nil, serviceCheckMessage)

	c.sender.Commit()

	return nil
}

func ntpFactory() check.Check {
	return &NTPCheck{}
}

func init() {
	core.RegisterCheck("ntp", ntpFactory)
}
