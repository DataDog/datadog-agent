// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apiimpl

import (
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func (server *apiServer) mountCapture(mux *http.ServeMux) error {
	if server.captureManager == nil {
		return nil
	}
	handler, err := server.captureManager.Handler(server.ipc.HTTPMiddleware, server.captureMetricSchedules)
	if err != nil {
		return err
	}
	mux.Handle("/agent/eudm-capture/", http.StripPrefix("/agent/eudm-capture", handler))
	return nil
}

func (server *apiServer) captureMetricSchedules() []telemetrycapture.MetricSchedule {
	if server.collector == nil {
		return nil
	}
	families := make(map[string]time.Duration, 7)
	server.collector.MapOverChecks(func(checks []check.Info) {
		for _, running := range checks {
			family := telemetrycapture.MetricCheckFamily(running.String())
			if family == "" {
				continue
			}
			if cadence := running.Interval(); cadence > families[family] {
				families[family] = cadence
			}
		}
	})
	schedules := make([]telemetrycapture.MetricSchedule, 0, len(families))
	for family, cadence := range families {
		schedules = append(schedules, telemetrycapture.MetricSchedule{Family: family, Cadence: cadence})
	}
	return schedules
}
