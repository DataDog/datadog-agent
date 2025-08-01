// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// tlmDogstatsd contains metrics that we want to capture from incoming dogstatsd traffic and send as agent telemetry.
var tlmDogstatsd = map[string]telemetry.Counter{
	"datadog.dogstatsd.client.bytes_sent": telemetry.NewCounterWithOpts("dogstatsd_client", "bytes_sent",
		[]string{"client", "client_version", "client_transport"}, "Intercepted from dogstatsd client", telemetry.DefaultOptions),
	"datadog.dogstatsd.client.bytes_dropped": telemetry.NewCounterWithOpts("dogstatsd_client", "bytes_dropped",
		[]string{"client", "client_version", "client_transport"}, "Intercepted from dogstatsd client", telemetry.DefaultOptions),
	"datadog.dogstatsd.client.packets_sent": telemetry.NewCounterWithOpts("dogstatsd_client", "packets_sent",
		[]string{"client", "client_version", "client_transport"}, "Intercepted from dogstatsd client", telemetry.DefaultOptions),
	"datadog.dogstatsd.client.packets_dropped": telemetry.NewCounterWithOpts("dogstatsd_client", "packets_dropped",
		[]string{"client", "client_version", "client_transport"}, "Intercepted from dogstatsd client", telemetry.DefaultOptions),
}

// newCOATBlocklist creates a block list to capture metrics coming in from dogstatsd that we want to capture.
// This currently only works with series.
func newAgentTelemFilterList() utilstrings.FilterList {
	return utilstrings.NewFilterList([]string{
		"datadog.dogstatsd.client.bytes_sent",
		"datadog.dogstatsd.client.bytes_dropped",
		"datadog.dogstatsd.client.packets_sent",
		"datadog.dogstatsd.client.packets_dropped"}, false)
}

// getDetailsFromSerie extracts the relevant details from the metrics serie
// that we can use to post to internal telemetry.
func getDetailsFromSerie(serie *metrics.Serie) (float64, []string) {
	tags := []string{"", "", ""}
	serie.Tags.ForEach(func(tag string) {
		t := strings.SplitN(tag, ":", 2)
		if len(t) > 1 {
			switch t[0] {
			case "client":
				tags[0] = t[1]
			case "client_version":
				tags[1] = t[1]
			case "client_transport":
				tags[2] = t[1]
			}
		}
	})

	var total float64
	for point := range serie.Points {
		total += serie.Points[point].Value
	}

	return total, tags
}

// addToAgentTelemetry converts the given metrics series into an agent metric.
// Currently all metrics we capturing are counters, so only counters are handled.
func addToAgentTelemetry(serie *metrics.Serie) {
	counter, ok := tlmDogstatsd[serie.Name]
	if ok {
		value, tags := getDetailsFromSerie(serie)
		counter.Add(value, tags...)
	}
}
