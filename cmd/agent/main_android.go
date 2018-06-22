// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build android

//go:generate go run ../../pkg/config/render_config.go agent ../../pkg/config/config_template.yaml ./dist/datadog.yaml

package main

import (
	ddapp "github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/event/touch"
)

var (
	green  float32
	touchX float32
	touchY float32
)

func main() {
	// Invoke the Agent
	app.Main(func(a app.App) {
		ddapp.StartAgent()
		var sz size.Event
		for e := range a.Events() {
			switch e := a.Filter(e).(type) {
			case lifecycle.Event:
				switch e.Crosses(lifecycle.StageVisible) {
				case lifecycle.CrossOn:
					log.Debug("DDAGENT --> now visible")
				case lifecycle.CrossOff:
					log.Debug("DDAGENT --> now not visible")
				}

				switch e.Crosses(lifecycle.StageFocused) {
				case lifecycle.CrossOn:
					log.Debug("DDAGENT --> now has focus")
				case lifecycle.CrossOff:
					log.Debug("DDAGENT --> now lost focus")
				}

				switch e.Crosses(lifecycle.StageAlive) {
				case lifecycle.CrossOn:
					log.Debug("DDAGENT --> app has been started")
				case lifecycle.CrossOff:
					log.Debug("DDAGENT --> is not alive")
				}

				switch e.Crosses(lifecycle.StageDead) {
				case lifecycle.CrossOn:
					log.Debug("DDAGENT --> app has died")
				case lifecycle.CrossOff:
					log.Debug("DDAGENT --> StageDead CrossOff?")
				}

			case size.Event:
				sz = e
				touchX = float32(sz.WidthPx / 2)
				touchY = float32(sz.HeightPx / 2)
			case paint.Event:
				a.Publish()

			case touch.Event:
				touchX = e.X
				touchY = e.Y
			}
		}
	})
}
