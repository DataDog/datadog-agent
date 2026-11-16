// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package io implements the io check.
package io

import (
	"regexp"

	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// SectorSize is used here to substitute non-exporeted from github.com/shirou/gopsutil/v4/disk package constant named "sectorSize"
	SectorSize = 512
	kB         = (1 << 10)
	// CheckName is the name of the io check.
	CheckName = "io"
)

// Configure the IOstats check
func (c *IOCheck) commonConfigure(senderManager sender.SenderManager, data integration.Data, initConfig integration.Data, source string, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, data, source, provider); err != nil {
		return err
	}

	conf := ioConfig{}
	err := yaml.Unmarshal([]byte(initConfig), &conf)
	if err != nil {
		return err
	}

	regex := ""
	if conf.DeviceExcludeRe != nil {
		regex = *conf.DeviceExcludeRe
	} else if conf.DeviceBlacklistRe != nil {
		log.Warn("'device_blacklist_re' has been deprecated, use 'device_exclude_re' instead")
		regex = *conf.DeviceBlacklistRe
	}
	if regex != "" {
		c.blacklist, err = regexp.Compile(regex)
		if err != nil {
			return err
		}
	}
	c.lowercaseDeviceTag = conf.LowercaseDeviceTag

	return nil
}

// ioConfig holds the init_config options of the io check. The regex options
// are pointers so that an explicitly empty device_exclude_re does not fall
// back to the deprecated device_blacklist_re, only an absent one does.
type ioConfig struct {
	DeviceExcludeRe    *string `yaml:"device_exclude_re"`
	DeviceBlacklistRe  *string `yaml:"device_blacklist_re"`
	LowercaseDeviceTag bool    `yaml:"lowercase_device_tag"`
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &IOCheck{
		CheckBase: core.NewCheckBase(CheckName),
	}
}
