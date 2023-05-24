// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package systray

import (
	"github.com/DataDog/datadog-agent/cmd/agent/windows/controlsvc"
)

func onRestart(s *systray) {
	if err := controlsvc.RestartService(); err != nil {
		s.log.Warnf("Failed to restart datadog service %v", err)
	}
}

func onStart(s *systray) {
	if err := controlsvc.StartService(); err != nil {
		s.log.Warnf("Failed to start datadog service %v", err)
	}
}

func onStop(s *systray) {
	if err := controlsvc.StopService(); err != nil {
		s.log.Warnf("Failed to stop datadog service %v", err)
	}
}
