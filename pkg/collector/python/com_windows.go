// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package python

import (
	"errors"

	"github.com/go-ole/go-ole"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// sFalse is S_FALSE: COM was already initialized on this thread with the
	// requested concurrency model. The call still takes a reference.
	sFalse = 0x00000001
	// rpcEChangedMode is RPC_E_CHANGED_MODE: COM was already initialized on
	// this thread with a different concurrency model. The call takes no
	// reference.
	rpcEChangedMode = 0x80010106
)

// initLoaderCOM initializes COM in the multithreaded model on the current OS
// thread and returns the function that releases it.
//
// Loads run on a locked OS thread taken from the Go runtime's pool, and that
// thread may have already run a Python check that initialized COM in the
// single-threaded model, for example through pythoncom.CoInitialize() in
// adodbapi. COM keeps a thread's concurrency model until the last
// CoUninitialize, so the multithreaded call then fails with
// RPC_E_CHANGED_MODE. COM is still usable on that thread, so the load goes
// ahead. The returned function must not call CoUninitialize in that case:
// this call took no reference, and releasing one would tear down the
// single-threaded COM the earlier check still relies on.
//
// Any other error is returned and fails the load.
func initLoaderCOM() (func(), error) {
	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	if err == nil {
		return ole.CoUninitialize, nil
	}

	var oleErr *ole.OleError
	if !errors.As(err, &oleErr) {
		return nil, err
	}
	switch uint32(oleErr.Code()) {
	case sFalse:
		return ole.CoUninitialize, nil
	case rpcEChangedMode:
		log.Debugf("COM is already initialized in the single-threaded model on this thread, loading the Python check without reinitializing it")
		return func() {}, nil
	default:
		return nil, err
	}
}
