// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package main

import (
	"github.com/DataDog/datadog-agent/cmd/agent/app"
	log "github.com/cihub/seelog"
)

func onRestart() {
	if err := app.StopService(app.ServiceName, true); err == nil {
		app.StartService(nil, nil)
	}

}
func onStart() {
	if err := app.StartService(nil, nil); err != nil {
		log.Warnf("Failed to start datadog service %v", err)
	}

}

func onStop() {
	if err := app.StopService(app.ServiceName, true); err != nil {
		log.Warnf("Failed to stop datadog service %v", err)
	}

}
