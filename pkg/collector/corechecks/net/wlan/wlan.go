// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	CheckName                    = "wlan"
	defaultMinCollectionInterval = 15
)

// WLANCheck monitors the status of the WLAN interface
type WLANCheck struct {
	core.CheckBase
}

func (c *WLANCheck) String() string {
	return "wlan"
}

// Run runs the check
func (c *WLANCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	wifiData, err := queryWiFiRSSI()
	if err != nil {
		log.Error(err)
		sender.Commit()
		return err
	}
	tags := []string{}
	tags = append(tags, "ssid:"+wifiData.ssid)
	tags = append(tags, "bssid:"+wifiData.bssid)

	sender.Gauge("wlan.rssi", float64(wifiData.rssi), "", tags)
	sender.Commit()
	return nil
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &WLANCheck{
		CheckBase: core.NewCheckBaseWithInterval(CheckName, time.Duration(defaultMinCollectionInterval)*time.Second),
	}
}
