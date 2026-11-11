// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build windows

// Package crashreport provides shared helpers for recording crash detection state
package crashreport

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	"golang.org/x/sys/windows/registry"
)

// WinCrashReporter is the helper object for getting/storing crash state
type WinCrashReporter struct {
	checkpointKey    string
	legacyHive       registry.Key
	legacyKey        string
	startupWarnCount int
	hasRunOnce       bool
	sysProbeClient   *sysprobeclient.CheckClient
}

const (
	maxStartupWarnings = 20
	reportedKey        = `lastReported`
	checkpointVersion  = 1
)

// crashCheckpoint is the on-disk format for crash deduplication state.
type crashCheckpoint struct {
	Version    int    `json:"version"`
	FileName   string `json:"file_name"`
	DateString string `json:"date_string"`
}

// NewWinCrashReporter creates a file-backed crash reporter. The legacy registry
// location is read only when importing a checkpoint left by an older Agent.
func NewWinCrashReporter(checkpointKey string, legacyHive registry.Key, legacyKey string) (*WinCrashReporter, error) {
	wcr := &WinCrashReporter{
		checkpointKey:  checkpointKey,
		legacyHive:     legacyHive,
		legacyKey:      legacyKey,
		sysProbeClient: sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"))),
	}
	return wcr, nil
}

// legacyReportedValue is used only to compare the old registry marker. There
// is no unambiguous way to split that marker back into its two fields.
func legacyReportedValue(wcs probe.WinCrashStatus) string {
	return fmt.Sprintf("%s_%s", wcs.FileName, wcs.DateString)
}

func (wcr *WinCrashReporter) writeCheckpoint(wcs probe.WinCrashStatus) error {
	value, err := json.Marshal(crashCheckpoint{
		Version:    checkpointVersion,
		FileName:   wcs.FileName,
		DateString: wcs.DateString,
	})
	if err != nil {
		return err
	}
	return persistentcache.WriteAtomic(wcr.checkpointKey, string(value))
}

func (wcr *WinCrashReporter) haveAlreadyReported(wcs probe.WinCrashStatus) bool {
	cacheKey := wcr.checkpointKey
	// A missing or empty checkpoint has no usable state; only a nonempty
	// file takes precedence over the legacy registry value.
	reportedVal, err := persistentcache.Read(cacheKey)
	if err != nil {
		log.Warnf("Cannot read crash checkpoint %s: %v", cacheKey, err)
		return false
	}
	if reportedVal != "" {
		// Compare only the fields used for deduplication; ignore version and
		// any unknown fields from future checkpoint formats.
		var saved crashCheckpoint
		if err := json.Unmarshal([]byte(reportedVal), &saved); err != nil {
			log.Warnf("Invalid crash checkpoint %s: %v", cacheKey, err)
			return false
		}
		return saved.FileName == wcs.FileName && saved.DateString == wcs.DateString
	}

	k, err := registry.OpenKey(wcr.legacyHive, wcr.legacyKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	legacyValue, _, err := k.GetStringValue(reportedKey)
	_ = k.Close()
	if err != nil || legacyValue == "" || legacyValue != legacyReportedValue(wcs) {
		return false
	}
	// A matching legacy marker can be converted using the current crash's
	// separate fields. Do not try to parse the old filename_date string.
	if err := wcr.writeCheckpoint(wcs); err != nil {
		log.Warnf("Cannot migrate crash checkpoint %s: %v", cacheKey, err)
	}
	return true
}

func (wcr *WinCrashReporter) setReported(wcs probe.WinCrashStatus) {
	if err := wcr.writeCheckpoint(wcs); err != nil {
		// The same crash may be reported again after restart if it cannot be saved.
		log.Warnf("Cannot save crash checkpoint %s: %v", wcr.checkpointKey, err)
	}
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

	crash, err := sysprobeclient.GetCheck[probe.WinCrashStatus](wcr.sysProbeClient, sysconfigtypes.WindowsCrashDetectModule)
	if err != nil {
		if sysprobeclient.IgnoreStartupError(err) == nil {
			return nil, nil
		}
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
