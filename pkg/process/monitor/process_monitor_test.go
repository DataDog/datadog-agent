// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package monitor

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util"
)

func TestProcessMonitorBasics(t *testing.T) {
	// Making sure we get the same process monitor if we call it twice.
	pm := GetProcessMonitor()
	pm2 := GetProcessMonitor()

	require.Equal(t, pm, pm2)

	// Sanity subscribing a callback.
	callback := &ProcessCallback{
		Event:    EXEC,
		Metadata: ANY,
		Callback: func(pid uint32) {},
	}
	unsubscribe, err := pm.Subscribe(callback)
	require.NoError(t, err)

	// Sanity subscribing a callback.
	callback2 := &ProcessCallback{
		Event:    EXEC,
		Metadata: ANY,
		Callback: func(pid uint32) {},
	}
	unsubscribe2, err := pm.Subscribe(callback2)
	require.NoError(t, err)

	// duplicated subscription should fail.
	_, err = pm.Subscribe(callback)
	require.Error(t, err)

	// making sure unsubscribe works and does not panic for the second unsubscription.
	unsubscribe()
	require.NotPanics(t, unsubscribe)
	unsubscribe2()
	require.NotPanics(t, unsubscribe2)
}

func TestProcessMonitorCallbacks(t *testing.T) {
	pm := GetProcessMonitor()

	numberOfExecs := 0
	numberOfExits := 0

	tmpFile, err := ioutil.TempFile("", "echo")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	err = util.CopyFile("/bin/echo", tmpFile.Name())
	require.NoError(t, err)

	require.NoError(t, os.Chmod(tmpFile.Name(), 0500))

	require.NoError(t, pm.Initialize())
	defer pm.Stop()
	callbackExec := &ProcessCallback{
		Event:    EXEC,
		Metadata: NAME,
		Regex:    regexp.MustCompile(path.Base(tmpFile.Name())),
		Callback: func(pid uint32) {
			numberOfExecs++
		},
	}
	callbackExit := &ProcessCallback{
		Event:    EXIT,
		Metadata: NAME, // we want only the captured Exec process
		Regex:    regexp.MustCompile(path.Base(tmpFile.Name())),
		Callback: func(pid uint32) {
			numberOfExits++
		},
	}

	unsubscribeExec, err := pm.Subscribe(callbackExec)
	require.NoError(t, err)
	unsubscribeExit, err := pm.Subscribe(callbackExit)
	require.NoError(t, err)

	require.NoError(t, exec.Command(tmpFile.Name(), "test").Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs == 1 && numberOfExits == 1
	}, time.Second, time.Millisecond*200, fmt.Sprintf("didn't capture exec %d and exit %d", numberOfExecs, numberOfExits))

	unsubscribeExit()
	require.NoError(t, exec.Command(tmpFile.Name()).Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs == 2 && numberOfExits == 1
	}, time.Second, time.Millisecond*200, fmt.Sprintf("didn't capture exec %d and exit %d", numberOfExecs, numberOfExits))

	unsubscribeExec()
	require.NoError(t, exec.Command(tmpFile.Name()).Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs == 2 && numberOfExits == 1
	}, time.Second, time.Millisecond*200, fmt.Sprintf("didn't capture exec %d and exit %d", numberOfExecs, numberOfExits))

}

func TestProcessMonitorRefcount(t *testing.T) {
	pm := GetProcessMonitor()
	require.Equal(t, pm.refcount, 0)
	err := pm.Initialize()
	require.Equal(t, pm.refcount, 1)
	require.NoError(t, err)
	pm.Stop()
	require.Equal(t, pm.refcount, 0)

	pm2 := GetProcessMonitor()

	numberOfExecs := 0
	callbackExec := &ProcessCallback{
		Event:    EXEC,
		Metadata: ANY,
		Callback: func(pid uint32) {
			numberOfExecs++
		},
	}
	_, err = pm.Subscribe(callbackExec)
	require.NoError(t, err)
	require.NoError(t, pm2.Initialize())
	require.Equal(t, pm.refcount, 1)

	oldNumberOfExecs := numberOfExecs
	require.NoError(t, exec.Command("/bin/echo").Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs > oldNumberOfExecs
	}, time.Second, time.Millisecond*200, fmt.Sprintf("didn't capture a new exec %d old %d", numberOfExecs, oldNumberOfExecs))

	require.NoError(t, pm2.Initialize())
	require.Equal(t, pm.refcount, 2)

	oldNumberOfExecs = numberOfExecs
	require.NoError(t, exec.Command("/bin/echo").Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs > oldNumberOfExecs
	}, time.Second, time.Millisecond*200, fmt.Sprintf("didn't capture a new exec %d old %d", numberOfExecs, oldNumberOfExecs))

	require.Equal(t, pm.refcount, 2)
	pm2.Stop()
	require.Equal(t, pm.refcount, 1)

	oldNumberOfExecs = numberOfExecs
	require.NoError(t, exec.Command("/bin/echo").Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs > oldNumberOfExecs
	}, time.Second, time.Millisecond*200, fmt.Sprintf("didn't capture a new exec %d old %d", numberOfExecs, oldNumberOfExecs))

	pm2.Stop()
	require.Equal(t, pm.refcount, 0)

	oldNumberOfExecs = numberOfExecs
	require.NoError(t, exec.Command("/bin/echo").Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs == oldNumberOfExecs
	}, time.Second, time.Millisecond*200, fmt.Sprintf("capture a new exec %d old %d", numberOfExecs, oldNumberOfExecs))
}
