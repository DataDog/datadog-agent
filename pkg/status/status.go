package status

import (
	"encoding/json"
	"expvar"
	"fmt"
	"strconv"
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

	statsString := getForwarderStatus([]byte(forwarderStats))
	statsString += getChecksStats([]byte(runnerStats), []byte(loaderStats))

	return statsString, nil
}

func getForwarderStatus(forwarderStatsJSON []byte) string {
	var formattedString = "===== Transactions =====\n\n"
	forwarderStats := make(map[string]interface{})

	json.Unmarshal(forwarderStatsJSON, &forwarderStats)
	if forwarderStats["TransactionsCreated"] == nil {
		return ""
	}
	var transactionStats = forwarderStats["TransactionsCreated"].(map[string]interface{})
	for name, stat := range transactionStats {
		formattedString += fmt.Sprintf("  %v: %v\n", name, stat)
	}
	formattedString += "\n\n"

	return formattedString
}

func getChecksStats(runnerStatsJSON []byte, loaderStatsJSON []byte) string {
	var formattedString = "===== Checks =====\n\n"
	var checkPrefix = "   "

	runnerStats := make(map[string]interface{})
	loaderStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats)
	json.Unmarshal(loaderStatsJSON, &loaderStats)

	if runnerStats["Checks"] == nil && loaderStats["Errors"] == nil {
		return ""
	}

	if runnerStats["Runs"] == 0 || runnerStats["Runs"] == nil {
		formattedString += "  === Running Checks ===\n\n"
		formattedString += checkPrefix + "No checks have run yet\n\n"
	}

	if runnerStats["Checks"] != nil {
		var runningHeader = false
		var checksStats = runnerStats["Checks"].(map[string]interface{})
		for _, m := range checksStats {
			if !runningHeader {
				runningHeader = true
				formattedString += "  === Running Checks ===\n\n"
			}
			var checkStats = m.(map[string]interface{})
			formattedString += fmt.Sprintf("%s== %s ==\n", checkPrefix, checkStats["CheckName"].(string))
			if checkStats["LastError"] != "" {
				formattedString += fmt.Sprintf("%sError: %s\n", checkPrefix, checkStats["LastError"].(string))
			}
			formattedString += checkPrefix + "Total Runs: "
			formattedString += strconv.FormatInt(int64(checkStats["TotalRuns"].(float64)), 10) + "\n"
			formattedString += "\n"
		}
	}

	if loaderStats["Errors"] != nil {
		var loadingHeader = false
		var loaderErrors = loaderStats["Errors"].(map[string]interface{})
		for checkName, errors := range loaderErrors {
			if !loadingHeader {
				loadingHeader = true
				formattedString += "  === Loading Errors ===\n\n"
			}
			formattedString += checkPrefix + "== " + checkName + " ==\n"
			for kind, err := range errors.(map[string]interface{}) {
				formattedString += checkPrefix + kind + ": " + err.(string) + "\n"
			}
			formattedString += "\n"
		}
	}

	return formattedString
}
