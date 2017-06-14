package system

import (
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

/*
#include <unistd.h>
#include <sys/types.h>
#include <stdlib.h>
*/
import "C"

func (c *IOCheck) String() string {
	return "IOCheck"
}

// Configure the IOstats check
func (c *IOCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	err := error(nil)

	blacklistRe := config.Datadog.GetString("device_blacklist_re")
	if blacklistRe != "" {
		c.blacklist, err = regexp.Compile(blacklistRe)
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
