// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package ssi

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// GetInstrumentationStatus returns the status of the APM auto-instrumentation on Windows.
func GetInstrumentationStatus() (status APMInstrumentationStatus, err error) {
	// IIS instrumentation: check applicationHost.config for Datadog .NET library
	iisCfgPath := filepath.Join(os.Getenv("windir"), "System32", "inetsrv", "config", "applicationHost.config")
	iisContent, iisErr := os.ReadFile(iisCfgPath)
	if iisErr != nil && !errors.Is(iisErr, os.ErrNotExist) {
		return status, fmt.Errorf("could not read applicationHost.config: %w", iisErr)
	}
	if bytes.Contains(iisContent, []byte("datadog-apm-library-dotnet")) {
		status.IISInstrumented = true
	}

	// Host instrumentation: check if the DDInjector kernel driver service is running
	running, svcErr := winutil.IsServiceRunning("DDInjector")
	if svcErr != nil && !errors.Is(svcErr, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return status, fmt.Errorf("could not check DDInjector service: %w", svcErr)
	}
	status.HostInstrumented = running

	return status, nil
}
