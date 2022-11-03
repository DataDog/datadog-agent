// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package main

import (
	"github.com/DataDog/datadog-agent/cmd/agent/windows/controlsvc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func onRestart() {
	if err := controlsvc.RestartService(); err != nil {
		log.Warnf("Failed to restart datadog service %v", err)
	}

}
func onStart() {
	if err := controlsvc.StartService(); err != nil {
		log.Warnf("Failed to start datadog service %v", err)
	}

}

func onStop() {
	if err := controlsvc.StopService(); err != nil {
		log.Warnf("Failed to stop datadog service %v", err)
	}

}
