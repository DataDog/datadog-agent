package system

import (
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

func (c *IOCheck) String() string {
	return "IOCheck"
}

// Configure the IOstats check
func (c *IOCheck) commonConfigure(data check.ConfigData, initConfig check.ConfigData) error {
	err := error(nil)

	conf := make(map[interface{}]interface{})

	err = yaml.Unmarshal([]byte(initConfig), &conf)
	if err != nil {
		return err
	}

	blacklistRe, ok := conf["device_blacklist_re"]
	if ok && blacklistRe != "" {
		if regex, ok := blacklistRe.(string); ok {
				c.blacklist, err = regexp.Compile(regex)
		}
	}
	return err
}

// InitSender initializes a sender
func (c *IOCheck) InitSender() {
	s, err := aggregator.GetSender(c.ID())
	if err != nil {
		log.Error(err)
		return
	}

	c.sender = s
}

// Interval returns the scheduling time for the check
func (c *IOCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *IOCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *IOCheck) Stop() {}
