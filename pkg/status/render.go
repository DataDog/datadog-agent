package status

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
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
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)

	stats := make(map[string]interface{})
	json.Unmarshal(data, &stats)
	forwarderStats := stats["forwarderStats"]
	runnerStats := stats["runnerStats"]
	loaderStats := stats["loaderStats"]
	aggregatorStats := stats["aggregatorStats"]
	title := fmt.Sprintf("Agent (v%s)", stats["version"])
	stats["title"] = title
	renderHeader(writer, stats)
	renderChecksStats(writer, runnerStats, loaderStats)
	renderForwarderStatus(writer, forwarderStats)
	renderAggregatorStatus(writer, aggregatorStats)

	b.String()

	return b.String(), nil
}

func renderHeader(w io.Writer, stats map[string]interface{}) {
	t := template.Must(template.New("header.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "header.tmpl")))
	err := t.Execute(os.Stdout, stats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderAggregatorStatus(w io.Writer, aggregatorStats interface{}) {
	t := template.Must(template.New("aggregator.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "aggregator.tmpl")))
	err := t.Execute(os.Stdout, aggregatorStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderForwarderStatus(w io.Writer, forwarderStats interface{}) {
	t := template.Must(template.New("forwarder.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "forwarder.tmpl")))
	err := t.Execute(os.Stdout, forwarderStats)
	if err != nil {
		fmt.Println(err)
	}
}

func renderChecksStats(w io.Writer, runnerStats interface{}, loaderStats interface{}) {
	checkStats := make(map[string]interface{})
	checkStats["RunnerStats"] = runnerStats
	checkStats["LoaderStats"] = loaderStats
	t := template.Must(template.New("checks.tmpl").Funcs(fmap).ParseFiles(filepath.Join(templateFolder, "checks.tmpl")))
	err := t.Execute(os.Stdout, checkStats)
	if err != nil {
		fmt.Println(err)
	}
}
