package system

import (
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"

	log "github.com/cihub/seelog"

	"gopkg.in/yaml.v2"
)

func (c *IOCheck) String() string {
	return "io"
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

func init() {
	core.RegisterCheck("io", ioFactory)
}

func ioFactory() check.Check {
	log.Debug("IOCheck factory")
	c := &IOCheck{}
	return c
}
