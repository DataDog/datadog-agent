package status

import (
	"encoding/json"
	"expvar"
	"fmt"
	"html/template"
	"os"
)

func GetStatus() (map[string]string, error) {
	stats := make(map[string]string)
	forwarderStats := expvar.Get("forwarder").String()
	stats["forwarderStats"] = forwarderStats
	runnerStats := expvar.Get("runner").String()
	stats["runnerStats"] = runnerStats
	loaderStats := expvar.Get("loader").String()
	stats["loaderStats"] = loaderStats

	return stats, nil
}

func FormatStatus(data []byte) (string, error) {
	stats := make(map[string]string)

	json.Unmarshal(data, &stats)

	forwarderStats := stats["forwarderStats"]
	runnerStats := stats["runnerStats"]
	loaderStats := stats["loaderStats"]

	fmt.Println("===== AGENT STATUS =====")

	getForwarderStatus([]byte(forwarderStats))
	getChecksStats([]byte(runnerStats), []byte(loaderStats))

	return "", nil
}

func getForwarderStatus(forwarderStatsJSON []byte) {
	forwarderStats := make(map[string]interface{})

	json.Unmarshal(forwarderStatsJSON, &forwarderStats)

	t := template.Must(template.New("forwarder.tmpl").Parse(forwarder))
	t.Execute(os.Stdout, forwarderStats)

}

func getChecksStats(runnerStatsJSON []byte, loaderStatsJSON []byte) {

	runnerStats := make(map[string]interface{})
	loaderStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats)
	json.Unmarshal(loaderStatsJSON, &loaderStats)
	checkStats := make(map[string]map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["LoaderStats"] = loaderStats

	t := template.Must(template.New("forwarder.tmpl").Parse(checks))
	t.Execute(os.Stdout, checkStats)
}
