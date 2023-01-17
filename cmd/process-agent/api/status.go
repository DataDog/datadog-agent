// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func statusHandler(w http.ResponseWriter, _ *http.Request) {
	log.Info("Got a request for the status. Making status.")

	// Get expVar server address
	ipcAddr, err := ddconfig.GetIPCAddress()
	if err != nil {
		writeError(err, http.StatusInternalServerError, w)
		_ = log.Warn("config error:", err)
		return
	}

	port := ddconfig.Datadog.GetInt("process_config.expvar_port")
	if port <= 0 {
		_ = log.Warnf("Invalid process_config.expvar_port -- %d, using default port %d\n", port, ddconfig.DefaultProcessExpVarPort)
		port = ddconfig.DefaultProcessExpVarPort
	}
	expvarEndpoint := fmt.Sprintf("http://%s:%d/debug/vars", ipcAddr, port)

	agentStatus, err := util.GetStatus(expvarEndpoint)
	if err != nil {
		_ = log.Warn("failed to get status from agent:", err)
		writeError(err, http.StatusInternalServerError, w)
		return
	}

	b, err := json.Marshal(agentStatus)
	if err != nil {
		_ = log.Warn("failed to serialize status response from agent:", err)
		writeError(err, http.StatusInternalServerError, w)
		return
	}

	_, err = w.Write(b)
	if err != nil {
		_ = log.Warn("received response from agent but failed write it to client:", err)
	}
}
