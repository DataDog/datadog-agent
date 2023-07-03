// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package wincrashdetect

import (
	"fmt"
	"net"
	"net/http"

	//"strings"
	"testing"

	//"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"

	//process_net "github.com/DataDog/datadog-agent/pkg/process/net"

	"golang.org/x/sys/windows/registry"
)

func createSystemProbeListener() (l net.Listener, close func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	return l, func() {
		_ = l.Close()
	}
}

func testSetup(t *testing.T) {
	// change the hive to hku for the test
	hive = registry.CURRENT_USER
	baseKey = `SOFTWARE\Datadog\unit_test\windows_crash_reporting`

	// clear the key before starting
	registry.DeleteKey(hive, baseKey)
}
func testCleanup() {
	cleanRegistryHistory()
}

func cleanRegistryHistory() {
	// clean up registry settings we left behind
	registry.DeleteKey(hive, baseKey)
}

func TestWinCrashReporting(t *testing.T) {

	listener, closefunc := createSystemProbeListener()
	defer closefunc()

	config.InitSystemProbeConfig(config.SystemProbe)

	mux := http.NewServeMux()
	server := http.Server{
		Handler: mux,
	}
	defer server.Close()

	sock := fmt.Sprintf("localhost:%d", listener.Addr().(*net.TCPAddr).Port)
	config.SystemProbe.Set("system_probe_config.sysprobe_socket", sock)

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

	mux.Handle("/windows_crash_detection/check", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		utils.WriteAsJSON(rw, p)
	}))
	mux.Handle("/debug/stats", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
	}))
	go server.Serve(listener)

	t.Run("test that no crash detected properly reports", func(t *testing.T) {
		testSetup(t)
		defer testCleanup()

		// set the return value handled in the check handler above
		p = &probe.WinCrashStatus{
			Success: true,
		}

		crashCheck := new(WinCrashDetect)
		mock := mocksender.NewMockSender(crashCheck.ID())

		err := crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 0)
		mock.AssertNumberOfCalls(t, "Commit", 0)
	})
	t.Run("test that a crash is properly reported", func(t *testing.T) {
		testSetup(t)
		defer testCleanup()
		p = &probe.WinCrashStatus{
			Success:    true,
			FileName:   `c:\windows\memory.dmp`,
			Type:       probe.DumpTypeAutomatic,
			DateString: `Fri Jun 30 15:33:05.086 2023 (UTC - 7:00)`,
			Offender:   `somedriver.sys`,
			BugCheck:   "0x00000007",
		}
		crashCheck := new(WinCrashDetect)
		mock := mocksender.NewMockSender(crashCheck.ID())

		expected := event.Event{
			Priority:       event.EventPriorityNormal,
			SourceTypeName: crashDetectCheckName,
			EventType:      crashDetectCheckName,
			Title:          formatTitle(*p),
			Text:           formatText(*p),
		}
		// set up to return from the event call when we get it
		mock.On("Event", expected).Return().Times(1)
		mock.On("Commit").Return().Times(1)
		// the first time we run, we should get the bug check notification

		err := crashCheck.Run()
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
		expected.Title = formatTitle(*p)
		expected.Text = formatText(*p)

		// set up to return from the event call when we get it
		mock.On("Event", expected).Return().Times(1)
		mock.On("Commit").Return().Times(1)
		crashCheck = new(WinCrashDetect)
		err = crashCheck.Run()
		assert.Nil(t, err)
		mock.AssertNumberOfCalls(t, "Gauge", 0)
		mock.AssertNumberOfCalls(t, "Rate", 0)
		mock.AssertNumberOfCalls(t, "Event", 2)
		mock.AssertNumberOfCalls(t, "Commit", 2)
	})
}
