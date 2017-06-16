package status

import (
	"encoding/json"
	"expvar"
	"fmt"
	"strconv"
)

func GetStatus() (string, error) {
	stats := make(map[string]string)
	forwarderStats := expvar.Get("forwarder").String()
	stats["forwarderStats"] = forwarderStats
	runnerStats := expvar.Get("runner").String()
	stats["runnerStats"] = runnerStats
	loaderStats := expvar.Get("loader").String()
	stats["loaderStats"] = loaderStats

	statsString := getForwarderStatus()
	statsString += getChecksStats()

	return statsString, nil
}

func getForwarderStatus() string {
	var formattedString = "===== Transactions =====\n\n"
	forwarderStats := make(map[string]interface{})
	forwarderStatsJSON := expvar.Get("forwarder").String()
	json.Unmarshal([]byte(forwarderStatsJSON), &forwarderStats)
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

func getChecksStats() string {
	var formattedString = "===== Checks =====\n\n"
	runnerStatsJSON := expvar.Get("runner").String()
	loaderStatsJSON := expvar.Get("loader").String()
	runnerStats := make(map[string]interface{})
	loaderStats := make(map[string]interface{})
	json.Unmarshal([]byte(runnerStatsJSON), &runnerStats)
	json.Unmarshal([]byte(loaderStatsJSON), &loaderStats)

	if runnerStats["Checks"] == nil && loaderStats["Errors"] == nil {
		return ""
	}

	if runnerStats["Checks"] != nil {
		var checksStats = runnerStats["Checks"].(map[string]interface{})
		for _, m := range checksStats {
			var checkStats = m.(map[string]interface{})
			formattedString += "  == " + checkStats["CheckName"].(string) + " ==\n"
			if checkStats["LastError"] != "" {
				formattedString += "  Error: " + checkStats["LastError"].(string) + "\n"
			}
			formattedString += "  Total Runs: "
			formattedString += strconv.FormatInt(int64(checkStats["TotalRuns"].(float64)), 10) + "\n"
			formattedString += "\n"
		}
	}

	if loaderStats["Errors"] != nil {
		var loaderErrors = loaderStats["Errors"].(map[string]interface{})
		for checkName, errors := range loaderErrors {
			formattedString += "  == " + checkName + " ==\n"
			for kind, err := range errors.(map[string]interface{}) {
				formattedString += "  " + kind + ": " + err.(string) + "\n"
			}
			formattedString += "\n"
		}
	}

	return formattedString
}
