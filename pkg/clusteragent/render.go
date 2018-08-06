package clusteragent

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/render"
)

// FormatStatus renders the go template using the json data for the Datadog Cluster Agent status.
func FormatStatus(data []byte) (string, error) {
	var b = new(bytes.Buffer)

	status := make(map[string]interface{})
	json.Unmarshal(data, &status)
	forwarderStats := status["forwarderStats"]
	runnerStats := status["runnerStats"]
	autoConfigStats := status["autoConfigStats"]
	checkSchedulerStats := status["checkSchedulerStats"]
	title := fmt.Sprintf("Datadog Cluster Agent (v%s)", status["version"])
	status["title"] = title
	render.Template(b, "header.tmpl", status)
	render.ChecksStats(b, runnerStats, nil, autoConfigStats, checkSchedulerStats, "")
	render.Template(b, "forwarder.tmpl", forwarderStats)

	return b.String(), nil
}

// FormatMetadataMap renders the go template using the json data for the metamap.
func FormatMetadataMap(data []byte) (string, error) {
	return render.FormatTemplate(data, "metadatamapper.tmpl")
}

// FormatHPAStatus renders the go template using the json data for custom metrics.
func FormatHPAStatus(data []byte) (string, error) {
	return render.FormatTemplate(data, "custommetricsprovider.tmpl")
}
