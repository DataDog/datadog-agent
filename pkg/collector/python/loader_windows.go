// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && windows

package python

import (
	"github.com/go-ole/go-ole"
)

const S_FALSE = 0x00000001

func platformLoaderPrep() error {
	// Initialize COM to multithreaded model
	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	if err != nil {
		oleCode := err.(*ole.OleError).Code()
		if oleCode != ole.S_OK && oleCode != S_FALSE {
			return err
		}
	}
	return nil
}

func platformLoaderDone() error {
	// UnInitialize COM
	ole.CoUninitialize()
	return nil
}
