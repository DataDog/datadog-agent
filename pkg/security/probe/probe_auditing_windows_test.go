// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package probe holds probe related files
package probe

import (
	"os"
	_ "os/exec"
	_ "path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	winacls "github.com/hectane/go-acl"
)

func processUntilAudit(t *testing.T, et *etwTester) {

	defer func() {
		et.loopExited <- struct{}{}
	}()
	et.loopStarted <- struct{}{}
	for {
		select {
		case <-et.stopLoop:
			return

		case n := <-et.notify:
			switch n.(type) {
			case *objectPermsChange:
				et.notifications = append(et.notifications, n)
				return
			}
		}
	}

}

func TestETWAuditNotifications(t *testing.T) {
	t.Skip("Skipping test that requires admin privileges")
	ebpftest.LogLevel(t, "info")
	ex, err := os.Executable()
	require.NoError(t, err, "could not get executable path")
	testfilename := ex + ".testfile"

	wp, err := createTestProbe()
	require.NoError(t, err)
	require.NotNil(t, wp)

	// teardownTestProe calls the stop function on etw, which will
	// in turn wait on wp.fimgw
	defer teardownTestProbe(wp)

	et := createEtwTester(wp)

	wp.fimwg.Add(1)
	go func() {
		defer wp.fimwg.Done()

		var once sync.Once
		mypid := os.Getpid()

		err := et.p.setupEtw(func(n interface{}, pid uint32) {
			once.Do(func() {
				close(et.etwStarted)
			})
			if pid != uint32(mypid) {
				return
			}
			select {
			case et.notify <- n:
				// message sent
			default:
			}
		})
		assert.NoError(t, err)
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		processUntilAudit(t, et)
	}()

	// wait until we're sure that the ETW listener is up and running.
	// as noted above, this _could_ cause an infinite deadlock if no notifications are received.
	// but, since we're getting the notifications from the entire system, we should be getting
	// a steady stream as soon as it's fired up.
	<-et.etwStarted
	<-et.loopStarted

	// create the test file
	f, err := os.Create(testfilename)
	assert.NoError(t, err)
	f.Close()

	// set up auditing on this directory
	/*
		dirpath := filepath.Dir(testfilename)

		// enable auditing

			pscommand := `$acl = new-object System.Security.AccessControl.DirectorySecurity;
			$accessrule = new-object System.Security.AccessControl.FileSystemAuditRule('everyone', 'modify', 'containerinherit, objectinherit', 'none', 'success');
			$acl.SetAuditRule($accessrule);
			$acl | set-acl -path`

			pscommand += dirpath + ";"

			cmd := exec.Command("powershell", "-Command", pscommand)
			assert.NoError(t, err)
			err = cmd.Run()
			assert.NoError(t, err)
	*/
	// this is kinda hokey.  ETW (which is what FIM is based on) takes an indeterminant amount of time to start up.
	// so wait around for it to start
	time.Sleep(2 * time.Second)
	err = winacls.Chmod(testfilename, 0600)
	assert.NoError(t, err)

	assert.Eventually(t, func() bool {
		select {
		case <-et.loopExited:
			return true
		}
		return false
	}, 10*time.Second, 250*time.Millisecond, "did not get notification")

	stopLoop(et, &wg)
	for _, n := range et.notifications {
		t.Logf("notification: %s", n)
	}

}
