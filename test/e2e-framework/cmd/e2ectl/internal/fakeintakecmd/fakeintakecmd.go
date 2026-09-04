// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package fakeintakecmd implements `e2ectl fakeintake ...` on top of the
// fakeintake Go client. It talks to the local fakeintake URL recorded in the
// environment metadata; no environment attach is needed.
package fakeintakecmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// Client builds the fakeintake client for a URL recorded in envstore metadata.
func Client(url string) (*fakeintakeclient.Client, error) {
	if url == "" {
		return nil, fmt.Errorf("environment has no fakeintake (was fakeintake disabled in its config?)")
	}
	// WithoutStrictFakeintakeIDCheck: the fakeintake ID check is a test-oriented
	// guard; the CLI may inspect payloads from any agent run.
	return fakeintakeclient.NewClient(url, fakeintakeclient.WithoutStrictFakeintakeIDCheck()), nil
}

// Names prints the metric names seen by the fakeintake.
func Names(url string, asJSON bool) error {
	c, err := Client(url)
	if err != nil {
		return err
	}
	names, err := c.GetMetricNames()
	if err != nil {
		return err
	}
	sort.Strings(names)
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(names)
	}
	if len(names) == 0 {
		fmt.Println("(no metrics yet)")
		return nil
	}
	for _, n := range names {
		fmt.Println(n)
	}
	return nil
}

// Metrics prints the payloads for a metric name (or all names with counts).
func Metrics(url, name string, asJSON bool) error {
	c, err := Client(url)
	if err != nil {
		return err
	}
	if name == "" {
		// One HTTP call for everything, parsed locally: a FilterMetrics call per
		// metric name re-fetches the full payload set every time and is
		// quadratic (minutes with a warm fakeintake — found the hard way).
		const metricsEndpoint = "/api/v2/series" // mirrors fakeintake client's private metricsEndpoint
		payloads, err := c.GetRawPayloads(metricsEndpoint)
		if err != nil {
			return err
		}
		counts := map[string]int{}
		for _, p := range payloads {
			series, err := aggregator.ParseMetricSeries(p)
			if err != nil {
				return err
			}
			for _, s := range series {
				counts[s.Metric]++
			}
		}
		if len(counts) == 0 {
			fmt.Println("(no metrics yet)")
			return nil
		}
		if asJSON {
			type count struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
			out := make([]count, 0, len(counts))
			for n, cnt := range counts {
				out = append(out, count{Name: n, Count: cnt})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
			return json.NewEncoder(os.Stdout).Encode(out)
		}
		for _, n := range sortedKeys(counts) {
			fmt.Printf("%-70s %d payloads\n", n, counts[n])
		}
		return nil
	}

	series, err := c.FilterMetrics(name)
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(series)
	}
	if len(series) == 0 {
		fmt.Printf("(no payloads for %q yet)\n", name)
		return nil
	}
	for _, s := range series {
		fmt.Printf("%s  host=%s  points=%d  tags=%v\n", name, hostOf(s), len(s.Points), s.Tags)
	}
	return nil
}

// hostOf extracts the host name from a series' resources.
func hostOf(s *aggregator.MetricSeries) string {
	for _, r := range s.Resources {
		if r.GetType() == "host" || r.Type == "host" {
			return r.Name
		}
	}
	return ""
}

// Health checks the fakeintake server.
func Health(url string) error {
	c, err := Client(url)
	if err != nil {
		return err
	}
	if err := c.GetServerHealth(); err != nil {
		return err
	}
	fmt.Println("fakeintake is healthy")
	return nil
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
