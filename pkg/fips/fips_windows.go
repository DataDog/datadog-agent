// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build goexperiment.systemcrypto && windows

package fips

import (
	"fmt"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

func Status() string {
	enabled, err := Enabled()
	if err != nil {
		return err
	}
	return strconv.FormatBool(enabled)
}

func Enabled() (bool, error) {
	// this is copied from how microsoft/go checks the windows registry that FIPS is enabled:
	//   https://github.com/microsoft/go/blob/d0f965f87c51211b3ea554f88e94b4c68116f5d1/eng/_util/cmd/run-builder/systemfips_windows.go#L17-L54
	key, _ := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return nil, err
	}

	enabled, enabledType, err := key.GetIntegerValue("Enabled")
	if err != nil {
		return nil, err
	}

	if enabledType != registry.DWORD {
		return nil, fmt.Errorf("unexpected FIPS algorithm policy Enabled key type: %v", enabledType)
	}

	return enabled == 1
}
