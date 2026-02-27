// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm)

package modules

import (
	"net/http"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	httpdebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/http/debugging"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func registerUSMCommonEndpoints(nt *networkTracer, httpMux *module.Router) {
	httpMux.HandleFunc("/debug/http_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.http.enabled") {
			writeDisabledProtocolMessage("http", w)
			return
		}
		id := utils.GetClientID(req)
		cs, cleanup, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}
		defer cleanup()

		utils.WriteAsJSON(req, w, httpdebugging.HTTP(cs.USMData.HTTP, cs.DNS), utils.GetPrettyPrintFromQueryParams(req))
	})

	httpMux.HandleFunc("/debug/usm_telemetry", telemetry.Handler)
}
