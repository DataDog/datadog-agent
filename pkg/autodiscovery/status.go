package autodiscovery

import (
	"encoding/json"
	"expvar"
)

// GetAutoConfigStatus returns status info for the auto config.
func GetAutoConfigStatus() map[string]interface{} {
	autoConfigStatsJSON := []byte(expvar.Get("autoconfig").String())
	autoConfigStats := make(map[string]interface{})
	json.Unmarshal(autoConfigStatsJSON, &autoConfigStats)
	return autoConfigStats
}
