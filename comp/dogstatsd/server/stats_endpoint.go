package server

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/api/api/utils"
)

func (s *server) writeStats(w http.ResponseWriter, _ *http.Request) {
	s.log.Info("Got a request for the Dogstatsd stats.")

	if !s.config.GetBool("use_dogstatsd") {
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]string{
			"error":      "Dogstatsd not enabled in the Agent configuration",
			"error_type": "no server",
		})
		w.WriteHeader(400)
		w.Write(body)
		return
	}

	if !s.config.GetBool("dogstatsd_metrics_stats_enable") {
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]string{
			"error":      "Dogstatsd metrics stats not enabled in the Agent configuration",
			"error_type": "not enabled",
		})
		w.WriteHeader(400)
		w.Write(body)
		return
	}

	// Weird state that should not happen: dogstatsd is enabled
	// but the server has not been successfully initialized.
	// Return no data.
	if !s.IsRunning() {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
		return
	}

	jsonStats, err := s.Debug.GetJSONDebugStats()
	if err != nil {
		utils.SetJSONError(w, s.log.Errorf("Error getting marshalled Dogstatsd stats: %s", err), 500)
		return
	}

	w.Write(jsonStats)
}
