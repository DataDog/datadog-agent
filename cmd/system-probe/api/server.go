// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"expvar"
	"fmt"
	"net/http"

	gorilla "github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StartServer starts the HTTP server for the system-probe, which registers endpoints from all enabled modules.
func StartServer(cfg *config.Config) error {
	conn, err := net.NewListener(cfg.SocketAddress)
	if err != nil {
		return fmt.Errorf("error creating IPC socket: %s", err)
	}

	mux := gorilla.NewRouter()
	err = module.Register(cfg, mux, modules.All)
	if err != nil {
		return fmt.Errorf("failed to create system probe: %s", err)
	}

	// Register stats endpoint
	mux.HandleFunc("/debug/stats", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
		stats := module.GetStats()
		utils.WriteAsJSON(w, stats)
	}))

	setupConfigHandlers(mux)

	// Module-restart handler
	mux.HandleFunc("/module-restart/{module-name}", restartModuleHandler).Methods("POST")

	mux.Handle("/debug/vars", http.DefaultServeMux)

	go func() {
		err = http.Serve(conn.GetListener(), mux)
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("error creating HTTP server: %s", err)
		}
	}()

	return nil
}

func init() {
	expvar.Publish("modules", expvar.Func(func() interface{} {
		return module.GetStats()
	}))
}
