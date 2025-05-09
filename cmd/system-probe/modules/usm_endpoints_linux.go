// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package modules

import (
	"net/http"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	httpdebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/http/debugging"
	kafkadebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/kafka/debugging"
	postgresdebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/debugging"
	redisdebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/redis/debugging"
	usmconsts "github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	usm "github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func registerUSMEndpoints(nt *networkTracer, httpMux *module.Router) {
	registerUSMCommonEndpoints(nt, httpMux)

	httpMux.HandleFunc("/debug/kafka_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.enable_kafka_monitoring") {
			writeDisabledProtocolMessage("kafka", w)
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

		utils.WriteAsJSON(w, kafkadebugging.Kafka(cs.Kafka), utils.GetPrettyPrintFromQueryParams(req))
	})

	httpMux.HandleFunc("/debug/postgres_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.enable_postgres_monitoring") {
			writeDisabledProtocolMessage("postgres", w)
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

		utils.WriteAsJSON(w, postgresdebugging.Postgres(cs.Postgres), utils.GetPrettyPrintFromQueryParams(req))
	})

	httpMux.HandleFunc("/debug/redis_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.enable_redis_monitoring") {
			writeDisabledProtocolMessage("redis", w)
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

		utils.WriteAsJSON(w, redisdebugging.Redis(cs.Redis), utils.GetPrettyPrintFromQueryParams(req))
	})

	httpMux.HandleFunc("/debug/http2_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.enable_http2_monitoring") {
			writeDisabledProtocolMessage("http2", w)
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

		utils.WriteAsJSON(w, httpdebugging.HTTP(cs.HTTP2, cs.DNS), utils.GetPrettyPrintFromQueryParams(req))
	})

	httpMux.HandleFunc("/debug/usm/traced_programs", usm.GetTracedProgramsEndpoint(usmconsts.USMModuleName))
	httpMux.HandleFunc("/debug/usm/blocked_processes", usm.GetBlockedPathIDEndpoint(usmconsts.USMModuleName))
	httpMux.HandleFunc("/debug/usm/clear_blocked", usm.GetClearBlockedEndpoint(usmconsts.USMModuleName))
	httpMux.HandleFunc("/debug/usm/attach-pid", usm.GetAttachPIDEndpoint(usmconsts.USMModuleName))
	httpMux.HandleFunc("/debug/usm/detach-pid", usm.GetDetachPIDEndpoint(usmconsts.USMModuleName))
}
