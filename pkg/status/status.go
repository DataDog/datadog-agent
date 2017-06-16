package status

import "expvar"

func GetStatus() (map[string]string, error) {
	stats := make(map[string]string)
	forwarderStats := expvar.Get("forwarder").String()
	stats["forwarderStats"] = forwarderStats
	runnerStats := expvar.Get("runner").String()
	stats["runnerStats"] = runnerStats

	return stats, nil
}
