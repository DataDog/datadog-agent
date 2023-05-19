// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package parser

import (
	"fmt"
)

type scmReader struct{}

// scmReader is a cross-platform compatibility wrapper around `winutil.SCMMonitor`.
// The non-windows version does nothing, and instead only exists so that we don't get compile errors.
func newSCMReader() *scmReader {
	return &scmReader{}
}

func (s *scmReader) getServiceInfo(pid uint64) (*WindowsServiceInfo, error) {
	return nil, fmt.Errorf("scm service info is only available on windows")
}
