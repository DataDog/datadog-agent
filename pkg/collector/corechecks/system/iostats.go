// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package system

import (
	"regexp"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const (
	// SectorSize is exported in github.com/shirou/gopsutil/disk (but not working!)
	SectorSize       = 512
	kB               = (1 << 10)
	iostatsCheckName = "io"
)

// Configure the IOstats check
func (c *IOCheck) commonConfigure(data check.ConfigData, initConfig check.ConfigData) error {
	conf := make(map[interface{}]interface{})

	err := yaml.Unmarshal([]byte(initConfig), &conf)
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

func init() {
	core.RegisterCheck(iostatsCheckName, ioFactory)
}

func ioFactory() check.Check {
	return &IOCheck{
		CheckBase: core.NewCheckBase(iostatsCheckName),
	}
}
