// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func setupCaptureHandlers(mux *http.ServeMux, deps module.FactoryDependencies) error {
	if deps.CaptureManager == nil {
		return nil
	}
	if deps.Ipc == nil {
		return telemetrycapture.ErrClosed
	}
	// System-probe's outer router has no global authentication. The shared
	// handler wraps every control and read route, including capabilities/status.
	handler, err := deps.CaptureManager.Handler(deps.Ipc.HTTPMiddleware)
	if err != nil {
		return err
	}
	handler = http.StripPrefix("/eudm-capture", handler)
	mux.Handle("/eudm-capture", handler)
	mux.Handle("/eudm-capture/", handler)
	return nil
}
