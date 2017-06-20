package status

import (
	"encoding/json"
	"expvar"
	"os"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/gohai/platform"
	log "github.com/cihub/seelog"
)

var timeFormat = "2006-01-02 15:04:05.000000 UTC"

// GetStatus grabs the status from expvar and puts it into a map
func GetStatus() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	stats, _ = expvarStats(stats)

	stats["version"] = version.AgentVersion
	hostname, err := util.GetHostname()
	if err != nil {
		log.Errorf("Error grabbing hostname for status: %v", err)
		stats["metadata"] = host.GetPayload("unknown")
	} else {
		stats["metadata"] = host.GetPayload(hostname)
	}

	platformPayload, err := new(platform.Platform).Collect()
	stats["pid"] = os.Getpid()
	stats["platform"] = platformPayload
	stats["hostinfo"] = host.GetStatusInformation()
	now := time.Now()
	stats["time"] = now.Format(timeFormat)

	if err != nil {
		return nil, err
	}

	return stats, nil
}

func expvarStats(stats map[string]interface{}) (map[string]interface{}, error) {
	forwarderStatsJSON := []byte(expvar.Get("forwarder").String())
	forwarderStats := make(map[string]interface{})
	json.Unmarshal(forwarderStatsJSON, &forwarderStats)
	stats["forwarderStats"] = forwarderStats

	runnerStatsJSON := []byte(expvar.Get("runner").String())
	runnerStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats)
	stats["runnerStats"] = runnerStats

	loaderStatsJSON := []byte(expvar.Get("loader").String())
	loaderStats := make(map[string]interface{})
	json.Unmarshal(loaderStatsJSON, &loaderStats)
	stats["loaderStats"] = loaderStats

	aggregatorStatsJSON := []byte(expvar.Get("aggregator").String())
	aggregatorStats := make(map[string]interface{})
	json.Unmarshal(aggregatorStatsJSON, &aggregatorStats)
	stats["aggregatorStats"] = aggregatorStats

	if expvar.Get("ntpOffset").String() != "" {
		stats["ntpOffset"], _ = strconv.ParseFloat(expvar.Get("ntpOffset").String(), 64)
	}

	return stats, nil
}
