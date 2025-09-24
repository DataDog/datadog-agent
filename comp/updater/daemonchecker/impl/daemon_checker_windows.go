// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemoncheckerimpl

import (
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func (c *checkerImpl) IsRunning() (bool, error) {
	manager, err := winutil.OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false, err
	}
	defer manager.Disconnect()

	service, err := winutil.OpenService(manager, "Datadog Installer", windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false, nil
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		return false, nil
	}

	return status.State == windows.SERVICE_RUNNING, nil
}
