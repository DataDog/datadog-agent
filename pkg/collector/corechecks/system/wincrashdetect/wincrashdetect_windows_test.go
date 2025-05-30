// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package wincrashdetect

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
)

const (
	// systemProbeTestPipeName is the test named pipe for system-probe
	systemProbeTestPipeName = `\\.\pipe\dd_system_probe_wincrash_test`
)

func testSetup(t *testing.T) {
	// change the hive to hku for the test
	hive = registry.CURRENT_USER
	baseKey = `SOFTWARE\Datadog\unit_test\windows_crash_reporting`

	// clear the key before starting
	_ = registry.DeleteKey(hive, baseKey)

	t.Cleanup(func() {
		// clean up registry settings we left behind
		_ = registry.DeleteKey(hive, baseKey)
	})
}

func TestWinCrashReporting(t *testing.T) {
	mockSysProbeConfig := configmock.NewSystemProbe(t)
	mockSysProbeConfig.SetWithoutSource("system_probe_config.enabled", true)
	mockSysProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", systemProbeTestPipeName)

	// The test named pipe allows the current user.
	listener, err := server.NewListenerForCurrentUser(systemProbeTestPipeName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	mux := http.NewServeMux()
	srv := http.Server{
		Handler: mux,
	}
	defer srv.Close()

	/*
	 * the underlying system probe connector is a singleton.  Therefore, we can't set up different
	 * tests that end up working on different ports; we have to have one for the duration of the test.
	 *
	 * so set up the handler functions to blindly return (as JSON) whatever the probe.WinCrashStatus (p)
	 * is, and then set it to the desire result before running each check.
	 *
	 * have individual checks wrapped in a `t.Run` for some sort of separation/clarity
	 */
	var p *probe.WinCrashStatus

	mux.Handle("/windows_crash_detection/check", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		utils.WriteAsJSON(rw, p, utils.CompactOutput)
	}))
	mux.Handle("/debug/stats", http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
	}))
	go srv.Serve(listener)

	t.Run("test that no crash detected properly reports", func(t *testing.T) {
		testSetup(t)

		// set the return value handled in the check handler above
		p = &probe.WinCrashStatus{
			StatusCode: probe.WinCrashStatusCodeSuccess,
		}

		check := newCheck()
		crashCheck := check.(*WinCrashDetect)
		mock := mocksender.NewMockSender(crashCheck.ID())
		err := crashCheck.Configure(mock.GetSenderManager(), 0, nil, nil, "")
		assert.NoError(t, err)

		err = crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 0)
		mock.AssertNumberOfCalls(t, "Commit", 0)
	})
	t.Run("test that a crash is properly reported", func(t *testing.T) {
		testSetup(t)
		p = &probe.WinCrashStatus{
			StatusCode: probe.WinCrashStatusCodeSuccess,
			FileName:   `c:\windows\memory.dmp`,
			Type:       probe.DumpTypeAutomatic,
			DateString: `Fri Jun 30 15:33:05.086 2023 (UTC - 7:00)`,
			Offender:   `somedriver.sys`,
			BugCheck:   "0x00000007",
		}
		check := newCheck()
		crashCheck := check.(*WinCrashDetect)
		mock := mocksender.NewMockSender(crashCheck.ID())
		err := crashCheck.Configure(mock.GetSenderManager(), 0, nil, nil, "")
		assert.NoError(t, err)

		expected := event.Event{
			Priority:       event.PriorityNormal,
			SourceTypeName: CheckName,
			EventType:      CheckName,
			AlertType:      event.AlertTypeError,
			Title:          formatTitle(p),
			Text:           formatText(p),
		}
		// set up to return from the event call when we get it
		mock.On("Event", expected).Return().Times(1)
		mock.On("Commit").Return().Times(1)
		// the first time we run, we should get the bug check notification

		err = crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 1)
		mock.AssertNumberOfCalls(t, "Commit", 1)

		// the second time we run, the check should not post another event for the same bug check

		err = crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 1)
		mock.AssertNumberOfCalls(t, "Commit", 1)

		// if we change the date string, we should not get another write on the same instance
		// (yes, I know june doesn't have 31 days).
		p.DateString = `Sat Jun 31 15:33:05.086 2023 (UTC - 7:00)`
		err = crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 1)
		mock.AssertNumberOfCalls(t, "Commit", 1)

		// if we now create a new instance of the check, we should see a new event because
		// it's a new bugcheck, different from the registry
		expected.Title = formatTitle(p)
		expected.Text = formatText(p)

		// set up to return from the event call when we get it
		mock.On("Event", expected).Return().Times(1)
		mock.On("Commit").Return().Times(1)

		check = newCheck()
		crashCheck = check.(*WinCrashDetect)
		err = crashCheck.Configure(mock.GetSenderManager(), 0, nil, nil, "")
		assert.Nil(t, err)
		err = crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 2)
		mock.AssertNumberOfCalls(t, "Commit", 2)
	})
}

func TestCrashReportingStates(t *testing.T) {
	mockSysProbeConfig := configmock.NewSystemProbe(t)
	mockSysProbeConfig.SetWithoutSource("system_probe_config.enabled", true)
	mockSysProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", systemProbeTestPipeName)

	var crashStatus *probe.WinCrashStatus

	// The test named pipe allows the current user.
	listener, err := server.NewListenerForCurrentUser(systemProbeTestPipeName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	mux := http.NewServeMux()
	srv := http.Server{
		Handler: mux,
	}
	defer srv.Close()

	cp, err := probe.NewWinCrashProbe(nil)
	assert.NotNil(t, cp)
	assert.Nil(t, err)

	wg := sync.WaitGroup{}

	// This will artificially delay the "parsing" to ensure the first check gets a "busy" status.
	delayedCrashDumpParser := func(wcs *probe.WinCrashStatus) {
		time.Sleep(4 * time.Second)

		assert.Equal(t, `c:\windows\memory.dmp`, wcs.FileName)
		assert.Equal(t, probe.DumpTypeAutomatic, wcs.Type)

		wcs.StatusCode = probe.WinCrashStatusCodeSuccess
		wcs.ErrString = crashStatus.ErrString
		wcs.DateString = crashStatus.DateString
		wcs.Offender = crashStatus.Offender
		wcs.BugCheck = crashStatus.BugCheck

		// Signal that the artificial delay is done.
		wg.Done()
	}

	// This ensures that no crash dump parsing should happen.
	noCrashDumpParser := func(_ *probe.WinCrashStatus) {
		assert.FailNow(t, "Should not parse")
	}

	mux.Handle("/windows_crash_detection/check", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		results := cp.Get()
		utils.WriteAsJSON(rw, results, utils.CompactOutput)
	}))
	mux.Handle("/debug/stats", http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
	}))
	go srv.Serve(listener)

	t.Run("test reporting a crash with a busy intermediate state", func(t *testing.T) {
		testSetup(t)

		check := newCheck()
		crashCheck := check.(*WinCrashDetect)
		mock := mocksender.NewMockSender(crashCheck.ID())
		err := crashCheck.Configure(mock.GetSenderManager(), 0, nil, nil, "")
		assert.NoError(t, err)

		crashStatus = &probe.WinCrashStatus{
			StatusCode: probe.WinCrashStatusCodeSuccess,
			FileName:   `c:\windows\memory.dmp`,
			Type:       probe.DumpTypeAutomatic,
			ErrString:  "",
			DateString: `Fri Jun 30 15:33:05.086 2023 (UTC - 7:00)`,
			Offender:   `somedriver.sys`,
			BugCheck:   "0x00000007",
		}

		// Test the 2-check response from crash reporting.
		cp.SetCachedSettings(crashStatus)
		probe.OverrideCrashDumpParser(delayedCrashDumpParser)

		// First run should be "busy" and not return an event yet.
		wg.Add(1)
		err = crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 0)
		mock.AssertNumberOfCalls(t, "Commit", 0)

		// Wait for the artificial delay to finish, plus a small time buffer.
		wg.Wait()
		time.Sleep(4 * time.Second)

		expected := event.Event{
			Priority:       event.PriorityNormal,
			SourceTypeName: CheckName,
			EventType:      CheckName,
			AlertType:      event.AlertTypeError,
			Title:          formatTitle(crashStatus),
			Text:           formatText(crashStatus),
		}

		mock.On("Event", expected).Return().Times(1)
		mock.On("Commit").Return().Times(1)

		// The result should be available now.
		err = crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 1)
		mock.AssertNumberOfCalls(t, "Commit", 1)
	})

	t.Run("test that no crash is reported", func(t *testing.T) {
		testSetup(t)

		check := newCheck()
		crashCheck := check.(*WinCrashDetect)
		mock := mocksender.NewMockSender(crashCheck.ID())
		err := crashCheck.Configure(mock.GetSenderManager(), 0, nil, nil, "")
		assert.NoError(t, err)

		noCrashStatus := &probe.WinCrashStatus{
			StatusCode: probe.WinCrashStatusCodeSuccess,
			FileName:   "",
		}

		// Test finding no crashes. The response should be immediate.
		cp.SetCachedSettings(noCrashStatus)
		probe.OverrideCrashDumpParser(noCrashDumpParser)
		err = crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 0)
		mock.AssertNumberOfCalls(t, "Commit", 0)
	})

	t.Run("test failure on reading crash settings", func(t *testing.T) {
		testSetup(t)

		check := newCheck()
		crashCheck := check.(*WinCrashDetect)
		mock := mocksender.NewMockSender(crashCheck.ID())
		err := crashCheck.Configure(mock.GetSenderManager(), 0, nil, nil, "")
		assert.NoError(t, err)

		failedStatus := &probe.WinCrashStatus{
			StatusCode: probe.WinCrashStatusCodeFailed,
			ErrString:  "Mocked failure",
		}

		// Test having a failure reading setings. The response should be immediate.
		cp.SetCachedSettings(failedStatus)
		probe.OverrideCrashDumpParser(noCrashDumpParser)
		err = crashCheck.Run()
		assert.NotNil(t, err)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 0)
		mock.AssertNumberOfCalls(t, "Commit", 0)
	})
}
