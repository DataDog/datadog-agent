// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"crypto/tls"
	"net/http"
	"time"

	configendpoint "github.com/DataDog/datadog-agent/comp/api/api/apiimpl/internal/config"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const ipcServerName string = "IPC API Server"
const ipcServerShortName string = "IPC"

func (server *apiServer) startIPCServer(ipcServerAddr string, tlsConfig *tls.Config, tmf observability.TelemetryMiddlewareFactory) (err error) {
	server.ipcListener, err = getListener(ipcServerAddr)
	if err != nil {
		return err
	}

	configEndpointMux := configendpoint.GetConfigEndpointMuxCore()
	configEndpointMux.Use(validateToken)

	ipcMux := http.NewServeMux()
	ipcMux.Handle(
		"/config/v1/",
		http.StripPrefix("/config/v1", configEndpointMux))

	// add some observability
	ipcMuxHandler := tmf.Middleware(ipcServerShortName)(ipcMux)
	ipcMuxHandler = observability.LogResponseHandler(ipcServerName)(ipcMuxHandler)

	ipcServer := &http.Server{
		Addr:      ipcServerAddr,
		Handler:   http.TimeoutHandler(ipcMuxHandler, time.Duration(pkgconfigsetup.Datadog().GetInt64("server_timeout"))*time.Second, "timeout"),
		TLSConfig: tlsConfig,
	}

	startServer(server.ipcListener, ipcServer, ipcServerName)

	return nil
}
