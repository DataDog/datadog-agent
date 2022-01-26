package api

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func statusHandler(w http.ResponseWriter, _ *http.Request) {
	log.Trace("Received status request from process agent")

	agentStatus, err := status.GetStatus()
	if err != nil {
		_ = log.Warn("failed to get status from agent:", agentStatus)
	}

	b, err := json.Marshal(agentStatus)
	if err != nil {
		_ = log.Warn("failed to serialize status response from agent:", err)
	}

	_, err = w.Write(b)
	if err != nil {
		_ = log.Warn("received response from agent but failed write it to client:", err)
	}
}
