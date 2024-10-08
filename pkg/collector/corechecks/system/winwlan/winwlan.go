// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

//nolint:revive // TODO(PLINT) Fix revive linter
package winwlan

import (
	"strconv"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/wlanapi"
	
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// CheckName is the name of the check
const CheckName = "winwlan"

// Config contains the configureation
type Config struct {
}

// Check doesn't need additional fields
type WLANCheck struct {
	core.CheckBase
	config Config
}


// Configure is called to configure the object prior to the first run
func (w *WLANCheck) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	if err := w.CommonConfigure(senderManager, initConfig, data, source); err != nil {
		return err
	}
	cf := Config{}

	w.config = cf
	log.Infof("WinWLAN check configured")
	return nil
}

// Run executes the check
func (c *WLANCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	wlanClient, err := wlanapi.OpenWLANHandle()
	if err != nil {
		log.Errorf("winwlan.Check: could not open WLAN handle: %s", err)
	}
	defer wlanClient.Close()

	ifaces, err := wlanClient.EnumNetworks()
	if err != nil {
		log.Errorf("winwlan.Check: could not enumerate WLAN interfaces: %s", err)
		return err
	}
	if len(ifaces) == 0 {
		return nil
	}
	for _, iface := range ifaces {
		for _, network := range iface.Networks {
			if len(network.SSID) == 0 {
				continue
			}
			tags := []string{}
			tags = append(tags, "interface:"+iface.InterfaceDescription)
			tags = append(tags, "ssid:"+network.SSID)
			tags = append(tags, "connectable:"+strconv.FormatBool(network.Connectable))
			tags = append(tags, "connected:"+strconv.FormatBool(network.Connected))

			sender.Gauge("system.wlan.signal_strength", float64(network.SignalStrength), "", tags)
		}
	}

	sender.Commit()

	return nil
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &WLANCheck{
		CheckBase: core.NewCheckBase(CheckName),
	}
}
