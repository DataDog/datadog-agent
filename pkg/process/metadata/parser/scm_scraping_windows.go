// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package parser

import (
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// scmScraper is a cross-platform compatability wrapper around `winutil.SCMMonitor`.
// The non-windows version does nothing, and instead only exists so that we don't get compile errors.
type scmScraper struct {
	scmMonitor *winutil.SCMMonitor
}

func newSCMScraper() *scmScraper {
	return &scmScraper{
		scmMonitor: winutil.GetServiceMonitor(),
	}
}

func (s *scmScraper) getServiceInfo(pid uint64) (*WindowsServiceInfo, error) {
	monitorServiceInfo, err := s.scmMonitor.GetServiceInfo(pid)
	if err != nil {
		return nil, err
	}

	if monitorServiceInfo == nil {
		return nil, nil
	}

	return &WindowsServiceInfo{
		ServiceName: monitorServiceInfo.ServiceName,
		DisplayName: monitorServiceInfo.DisplayName,
	}, nil
}
