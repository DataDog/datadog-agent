// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package python

import (
	"errors"
	"runtime"
	"testing"

	"github.com/go-ole/go-ole"
	"github.com/stretchr/testify/require"
)

// requireCOMInit asserts the result of CoInitializeEx: nil for S_OK, or the
// HRESULT carried by the returned error.
func requireCOMInit(t *testing.T, err error, want uint32) {
	t.Helper()
	if want == 0 {
		require.NoError(t, err)
		return
	}
	var oleErr *ole.OleError
	require.True(t, errors.As(err, &oleErr), "expected an OLE error, got %v", err)
	require.Equal(t, want, uint32(oleErr.Code()))
}

func lockThread(t *testing.T) {
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
}

func TestInitLoaderCOMOnUninitializedThread(t *testing.T) {
	lockThread(t)

	release, err := initLoaderCOM()
	require.NoError(t, err)
	release()

	// The loader's reference was the only one, so the thread is back to no COM
	// and can take either model.
	requireCOMInit(t, ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED), 0)
	ole.CoUninitialize()
}

func TestInitLoaderCOMOnMultithreadedThread(t *testing.T) {
	lockThread(t)
	requireCOMInit(t, ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED), 0)
	defer ole.CoUninitialize()

	release, err := initLoaderCOM()
	require.NoError(t, err)
	release()

	// The earlier multithreaded reference is still held.
	requireCOMInit(t, ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED), sFalse)
	ole.CoUninitialize()
}

// A Python check such as sqlserver with adodbapi initializes COM in the
// single-threaded model on whichever OS thread runs it. A later load on that
// thread must still succeed and must not release the check's reference.
func TestInitLoaderCOMOnSingleThreadedThread(t *testing.T) {
	lockThread(t)
	requireCOMInit(t, ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED), 0)
	defer ole.CoUninitialize()

	// The multithreaded call alone fails on this thread.
	requireCOMInit(t, ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED), rpcEChangedMode)

	release, err := initLoaderCOM()
	require.NoError(t, err)
	release()

	// Still single-threaded: another single-threaded call reports S_FALSE
	// instead of S_OK.
	requireCOMInit(t, ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED), sFalse)
	ole.CoUninitialize()
}
