// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build windows

// Package crashreport provides shared helpers for recording crash detection state
package crashreport

import (
	"fmt"
	"net/http"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	"golang.org/x/sys/windows/registry"
)

// WinCrashReporter is the helper object for getting/storing crash state
type WinCrashReporter struct {
	hive             registry.Key
	baseKey          string
	startupWarnCount int
	hasRunOnce       bool
	sysProbeClient   *http.Client
}

const (
	maxStartupWarnings = 20
	reportedKey        = `lastReported`
)

// NewWinCrashReporter creates the object for checking/setting the windows
// crash registry keys
func NewWinCrashReporter(hive registry.Key, key string) (*WinCrashReporter, error) {
	wcr := &WinCrashReporter{
		hive:           hive,
		baseKey:        key,
		sysProbeClient: sysprobeclient.Get(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
	}
	return wcr, nil
}

func (wcr *WinCrashReporter) generateReportedValue(wcs probe.WinCrashStatus) string {
	return fmt.Sprintf("%s_%s", wcs.FileName, wcs.DateString)
}

func (wcr *WinCrashReporter) haveAlreadyReported(wcs probe.WinCrashStatus) bool {

	newval := wcr.generateReportedValue(wcs)

	k, err := registry.OpenKey(wcr.hive, wcr.baseKey, registry.QUERY_VALUE)
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

func (wcr *WinCrashReporter) setReported(wcs probe.WinCrashStatus) {
	newval := wcr.generateReportedValue(wcs)

	k, _, err := registry.CreateKey(wcr.hive, wcr.baseKey, registry.ALL_ACCESS)
	if err != nil {
		// key not even there
		return
	}
	defer k.Close()
	// if we can't set the value, there's nothing we can do.  On next agent
	// start, the same crash will be reported if the file is still there.
	_ = k.SetStringValue(reportedKey, newval)
}

func (wcr *WinCrashReporter) handleStartupError(err error) error {
	if retry.IsErrWillRetry(err) {
		wcr.startupWarnCount++
		// this is an expected error, the check run will occur before the system probe
		// comes up.  However, it should resolve.  If the number of these exceeds a
		// given threshold, report as an error anyway
		if wcr.startupWarnCount > maxStartupWarnings {
			return err
		}
		return nil
	}
	return err
}

// CheckForCrash uses the system probe crash module to check for a crash
func (wcr *WinCrashReporter) CheckForCrash() (*probe.WinCrashStatus, error) {
	if wcr.hasRunOnce {
		return nil, nil
	}

	crash, err := sysprobeclient.GetCheck[probe.WinCrashStatus](wcr.sysProbeClient, sysconfig.WindowsCrashDetectModule)
	if err != nil {
		return nil, wcr.handleStartupError(err)
	}

	// Crash dump processing is not done yet, nothing to send at the moment. Try later.
	if crash.StatusCode == probe.WinCrashStatusCodeBusy {
		log.Infof("Crash dump processing is busy")
		return nil, nil
	}

	/*
	 * originally did this with a sync.once.  The problem is the check is run prior to the
	 * system probe being successfully started.  This is OK; we just need to detect the BSOD
	 * on first available run.
	 *
	 * so keep running the check until we are able to talk to system probe, after which
	 * we don't need to run any more
	 */
	wcr.hasRunOnce = true
	if crash.StatusCode == probe.WinCrashStatusCodeFailed {
		return nil, fmt.Errorf("Error getting crash data %s", crash.ErrString)
	}

	if len(crash.FileName) == 0 {
		// no crash data present.  this is actually good news.  We don't need to do anything
		// else
		return nil, nil
	}

	if wcr.haveAlreadyReported(crash) {
		log.Infof("Not reporting event on already reported crash")
		return nil, nil
	}
	wcr.setReported(crash)
	return &crash, nil

}
