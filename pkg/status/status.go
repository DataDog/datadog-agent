package status

import (
	"encoding/json"
	"expvar"
	"fmt"
	"html/template"
	"os"
	"strings"
)

var fmap = template.FuncMap{
	"doNotEscape":        doNotEscape,
	"lastError":          lastError,
	"lastErrorTraceback": lastErrorTraceback,
	"lastErrorMessage":   lastErrorMessage,
	"pythonLoaderError":  pythonLoaderError,
}

func GetStatus() (map[string]string, error) {
	stats := make(map[string]string)
	forwarderStats := expvar.Get("forwarder").String()
	stats["forwarderStats"] = forwarderStats
	runnerStats := expvar.Get("runner").String()
	stats["runnerStats"] = runnerStats
	loaderStats := expvar.Get("loader").String()
	stats["loaderStats"] = loaderStats
	aggregatorStats := expvar.Get("aggregator").String()
	stats["aggregatorStats"] = aggregatorStats

	return stats, nil
}

func FormatStatus(data []byte) (string, error) {
	stats := make(map[string]string)

	json.Unmarshal(data, &stats)

	forwarderStats := stats["forwarderStats"]
	runnerStats := stats["runnerStats"]
	loaderStats := stats["loaderStats"]
	aggregatorStats := stats["aggregatorStats"]

	fmt.Println("===== AGENT STATUS =====")

	getForwarderStatus([]byte(forwarderStats))
	getChecksStats([]byte(runnerStats), []byte(loaderStats))
	getAggregatorStatus([]byte(aggregatorStats))

	return "", nil
}

func getAggregatorStatus(aggregatorStatsJSON []byte) {
	aggregatorStats := make(map[string]interface{})

	json.Unmarshal(aggregatorStatsJSON, &aggregatorStats)

	t := template.Must(template.New("aggregator.tmpl").Funcs(fmap).Parse(aggregator))
	err := t.Execute(os.Stdout, aggregatorStats)
	if err != nil {
		fmt.Println(err)
	}
}

func getForwarderStatus(forwarderStatsJSON []byte) {
	forwarderStats := make(map[string]interface{})

	json.Unmarshal(forwarderStatsJSON, &forwarderStats)

	t := template.Must(template.New("forwarder.tmpl").Funcs(fmap).Parse(forwarder))
	err := t.Execute(os.Stdout, forwarderStats)
	if err != nil {
		fmt.Println(err)
	}
}

func getChecksStats(runnerStatsJSON []byte, loaderStatsJSON []byte) {

	runnerStats := make(map[string]interface{})
	loaderStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats)
	json.Unmarshal(loaderStatsJSON, &loaderStats)
	// fmt.Println(runnerStats["Checks"])

	// for check, stats := range runnerStats["Checks"].(map[string]interface{}) {
	// 	fmt.Println(check)
	// 	fmt.Println(stats)
	// }

	checkStats := make(map[string]map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["LoaderStats"] = loaderStats

	t := template.Must(template.New("checks.tmpl").Funcs(fmap).Parse(checks))
	err := t.Execute(os.Stdout, checkStats)
	if err != nil {
		fmt.Println(err)
	}
}

func doNotEscape(value string) template.HTML {
	return template.HTML(value)
}

func pythonLoaderError(value string) template.HTML {
	value = strings.Replace(value, "', '", "", -1)
	value = strings.Replace(value, "['", "", -1)
	value = strings.Replace(value, "\\n']", "", -1)
	value = strings.Replace(value, "']", "", -1)
	value = strings.Replace(value, "\\n", "\n      ", -1)
	var loaderErrorArray []string
	json.Unmarshal([]byte(value), &loaderErrorArray)
	return template.HTML(value)
}

func lastError(value string) template.HTML {
	return template.HTML(value)
}

func lastErrorTraceback(value string) template.HTML {
	var lastErrorArray []map[string]string

	json.Unmarshal([]byte(value), &lastErrorArray)
	lastErrorArray[0]["traceback"] = strings.Replace(lastErrorArray[0]["traceback"], "\n", "\n    ", -1)
	return template.HTML(lastErrorArray[0]["traceback"])
}

func lastErrorMessage(value string) template.HTML {
	var lastErrorArray []map[string]string

	json.Unmarshal([]byte(value), &lastErrorArray)
	return template.HTML(lastErrorArray[0]["message"])
}
