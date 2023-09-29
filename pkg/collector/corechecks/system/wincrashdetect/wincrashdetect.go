// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build windows

// Package wincrashdetect implements the windows crash detection check
package wincrashdetect

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/crashreport"
	"golang.org/x/sys/windows/registry"
	yaml "gopkg.in/yaml.v2"
)

const (
	crashDetectCheckName = "wincrashdetect"
)

var (
	// these are vars and not consts so that they can be overridden in
	// the unit tests.
	hive    = registry.LOCAL_MACHINE
	baseKey = `SOFTWARE\Datadog\Datadog Agent\windows_crash_reporting`
)

// WinCrashConfig is the configuration options for this check
// it is exported so that the yaml parser can read it.
type WinCrashConfig struct {
	Enabled bool `yaml:"enabled"` // placeholder for config
}

// WinCrashDetect is the object representing the check
type WinCrashDetect struct {
	core.CheckBase
	instance *WinCrashConfig
	reporter *crashreport.WinCrashReporter
}

func init() {
	core.RegisterCheck(crashDetectCheckName, crashDetectFactory)
}

func crashDetectFactory() check.Check {
	return &WinCrashDetect{
		CheckBase: core.NewCheckBase(crashDetectCheckName),
		instance:  &WinCrashConfig{},
	}
}

// Parse parses the check configuration
func (c *WinCrashConfig) Parse(data []byte) error {
	// default values
	c.Enabled = true

	return yaml.Unmarshal(data, c)
}

// Configure accepts configuration
func (wcd *WinCrashDetect) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := wcd.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	wcd.reporter, err = crashreport.NewWinCrashReporter(hive, baseKey)
	if err != nil {
		return err
	}
	return wcd.instance.Parse(initConfig)
}

// Run is called each time the scheduler runs this particular check.
func (wcd *WinCrashDetect) Run() error {

	crash, err := wcd.reporter.CheckForCrash()
	if err != nil {
		return err
	}
	if crash == nil {
		// no crash to send
		return nil
	}

	sender, err := wcd.GetSender()
	if err != nil {
		return err
	}
	ev := event.Event{
		Priority:       event.EventPriorityNormal,
		SourceTypeName: crashDetectCheckName,
		EventType:      crashDetectCheckName,
		Title:          formatTitle(crash),
		Text:           formatText(crash),
		AlertType:      event.EventAlertTypeError,
	}
	log.Infof("Sending event %v", ev)
	sender.Event(ev)
	sender.Commit()
	return nil
}

func formatTitle(c *probe.WinCrashStatus) string {
	return "A Windows system crash was detected"
}

func formatText(c *probe.WinCrashStatus) string {
	baseString := `A system crash was detected.
	The crash occurred at %s.
	The offending moudule is %s.
	The bugcheck code is %s`
	return fmt.Sprintf(baseString, c.DateString, c.Offender, c.BugCheck)
}
