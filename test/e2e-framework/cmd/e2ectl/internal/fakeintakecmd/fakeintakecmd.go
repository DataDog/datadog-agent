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
		names, err := c.GetMetricNames()
		if err != nil {
			return err
		}
		sort.Strings(names)
		type count struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		counts := make([]count, 0, len(names))
		for _, n := range names {
			series, err := c.FilterMetrics(n)
			if err != nil {
				return err
			}
			counts = append(counts, count{Name: n, Count: len(series)})
		}
		if asJSON {
			return json.NewEncoder(os.Stdout).Encode(counts)
		}
		if len(counts) == 0 {
			fmt.Println("(no metrics yet)")
			return nil
		}
		for _, c := range counts {
			fmt.Printf("%-70s %d payloads\n", c.Name, c.Count)
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
