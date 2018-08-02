package forwarder

import (
	"encoding/json"
	"expvar"
)

// GetStatus returns status info for the forwarder.
func GetStatus() map[string]interface{} {
	forwarderStatsJSON := []byte(expvar.Get("forwarder").String())
	forwarderStats := make(map[string]interface{})
	json.Unmarshal(forwarderStatsJSON, &forwarderStats)
	return forwarderStats
}
