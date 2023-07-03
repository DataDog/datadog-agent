// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.
//go:build windows

package wincrashdetect

import (
	"fmt"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	dd_config "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	process_net "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"golang.org/x/sys/windows/registry"
	yaml "gopkg.in/yaml.v2"
)

const (
	crashDetectCheckName = "wincrashdetect"
	maxStartupWarnings   = 20
	reportedKey          = `lastReported`
)

var (
	// these are vars and not consts so that they can be overridden in
	// the unit tests.
	hive    = registry.LOCAL_MACHINE
	baseKey = `SOFTWARE\Datadog\Datadog Agent\windows_crash_reporting`
)

// Config is the configuration options for this check
// it is exported so that the yaml parser can read it.
type WinCrashConfig struct {
	Enabled bool `yaml:"enabled"` // placeholder for config
}

type WinCrashDetect struct {
	core.CheckBase
	instance         *WinCrashConfig
	hasRunOnce       bool
	startupWarnCount int
}

func init() {
	core.RegisterCheck(crashDetectCheckName, crashDetectFactory)
}

func crashDetectFactory() check.Check {
	return &WinCrashDetect{
		CheckBase:  core.NewCheckBase(crashDetectCheckName),
		instance:   &WinCrashConfig{},
		hasRunOnce: false,
	}
}

// Parse parses the check configuration
func (c *WinCrashConfig) Parse(data []byte) error {
	// default values
	c.Enabled = true

	return yaml.Unmarshal(data, c)
}

func (wcd *WinCrashDetect) Configure(integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	err := wcd.CommonConfigure(integrationConfigDigest, initConfig, config, source)
	if err != nil {
		return err
	}

	return wcd.instance.Parse(config)
}

func (wcd *WinCrashDetect) handleStartupError(err error) error {
	if retry.IsErrWillRetry(err) {
		wcd.startupWarnCount++
		// this is an expected error, the check run will occur before the system probe
		// comes up.  However, it should resolve.  If the number of these exceeds a
		// given threshold, report as an error anyway
		if wcd.startupWarnCount > maxStartupWarnings {
			return err
		}
		return nil
	}
	return err
}
func (wcd *WinCrashDetect) Run() error {

	// only do this once; there's no point in doing it after the first one
	if wcd.hasRunOnce {
		log.Infof("check already run")
		return nil
	}
	sysProbeUtil, err := process_net.GetRemoteSystemProbeUtil(
		dd_config.SystemProbe.GetString("system_probe_config.sysprobe_socket"))
	if err != nil {
		return wcd.handleStartupError(err)
	}

	data, err := sysProbeUtil.GetCheck(sysconfig.WindowsCrashDetectModule)
	if err != nil {
		return wcd.handleStartupError(err)
	}
	crash, ok := data.(probe.WinCrashStatus)
	if !ok {
		return fmt.Errorf("Raw data has incorrect type")
	}
	/*
	 * originally did this with a sync.once.  The problem is the check is run prior to the
	 * system probe being successfully started.  This is OK; we just need to detect the BSOD
	 * on first available run.
	 *
	 * so keep running the check until we are able to talk to system probe, after which
	 * we don't need to run any more
	 */
	wcd.hasRunOnce = true
	if !crash.Success {
		return fmt.Errorf("Error getting crash data %s", crash.ErrString)
	}

	if len(crash.FileName) == 0 {
		// no crash data present.  this is actually good news.  We don't need to do anything
		// else
		return nil
	}

	if haveAlreadyReported(crash) {
		log.Infof("Not reporting event on already reported crash")
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
	}
	log.Infof("Sending event %v", ev)
	sender.Event(ev)
	sender.Commit()
	setReported(crash)
	return nil
}

func generateReportedValue(wcs probe.WinCrashStatus) string {
	return fmt.Sprintf("%s_%s", wcs.FileName, wcs.DateString)
}
func haveAlreadyReported(wcs probe.WinCrashStatus) bool {

	newval := generateReportedValue(wcs)

	k, err := registry.OpenKey(hive, baseKey, registry.QUERY_VALUE)
	if err != nil {
		// key not even there
		return false
	}
	defer k.Close()
	reportedval, _, err := k.GetStringValue(reportedKey)
	if err != nil {
		return false
	}
	if newval == reportedval {
		return true
	}
	return false
}

func setReported(wcs probe.WinCrashStatus) {
	newval := generateReportedValue(wcs)

	k, _, err := registry.CreateKey(hive, baseKey, registry.ALL_ACCESS)
	if err != nil {
		// key not even there
		return
	}
	defer k.Close()
	// if we can't set the value, there's nothing we can do.  On next agent
	// start, the same crash will be reported if the file is still there.
	_ = k.SetStringValue(reportedKey, newval)
}

func formatTitle(c probe.WinCrashStatus) string {
	return "A Windows system crash was detected"
}

func formatText(c probe.WinCrashStatus) string {
	baseString := `A system crash was detected.
	The crash occurred at %s.
	The offending moudule is %s.
	The bugcheck code is %s`
	return fmt.Sprintf(baseString, c.DateString, c.Offender, c.BugCheck)
}
