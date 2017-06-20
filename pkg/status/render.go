package status

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/kardianos/osext"
)

var (
	here, _        = osext.ExecutableFolder()
	fmap           template.FuncMap
	templateFolder string
)

func init() {
	templateFolder = filepath.Join(here, "dist", "templates")
}

// FormatStatus takes a json bytestring and prints out the formatted statuspage
func FormatStatus(data []byte) (string, error) {
	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats)
	forwarderStats := stats["forwarderStats"]
	runnerStats := stats["runnerStats"]
	loaderStats := stats["loaderStats"]
	aggregatorStats := stats["aggregatorStats"]
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title
	renderHeader(stats)
	renderChecksStats(runnerStats, loaderStats)
	renderForwarderStatus(forwarderStats)
	renderAggregatorStatus(aggregatorStats)

	return "", nil
}

func renderHeader(stats map[string]interface{}) {
	t := template.Must(template.New("header.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "header.tmpl")))
	err := t.Execute(os.Stdout, stats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderAggregatorStatus(aggregatorStats interface{}) {
	t := template.Must(template.New("aggregator.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "aggregator.tmpl")))
	err := t.Execute(os.Stdout, aggregatorStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderForwarderStatus(forwarderStats interface{}) {
	t := template.Must(template.New("forwarder.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "forwarder.tmpl")))
	err := t.Execute(os.Stdout, forwarderStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderChecksStats(runnerStats interface{}, loaderStats interface{}) {
	checkStats := make(map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["LoaderStats"] = loaderStats
	t := template.Must(template.New("checks.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "checks.tmpl")))
	err := t.Execute(os.Stdout, checkStats)
	if err != nil {
		fmt.Println(err)
	}
}
