package api

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/status"
)

func statusHandler(w http.ResponseWriter, _ *http.Request) {
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
