// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build benchmarking

package metrics

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/flags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Configure creates a statsd client for the given agent's configuration, using the specified global tags.
func Configure(cfg *config.AgentConfig, tags []string) error {
	f, err := os.Create(flags.StatsOut)
	if err != nil {
		return err
	}
	log.Infof("Outputting metrics to %q", flags.StatsOut)
	Client = &captureClient{
		f:    f,
		cfg:  cfg,
		tags: tags,
	}
	return nil
}

type captureClient struct {
	mu sync.Mutex // guards f
	f  *os.File

	lines uint64 // total lines written
	cfg   *config.AgentConfig
	tags  []string
}

// Gauge implements Client.
func (c *captureClient) Gauge(name string, value float64, tags []string, rate float64) error {
	return c.write("gauge", name, formatFloat(value), tags)
}

// Count implements Client.
func (c *captureClient) Count(name string, value int64, tags []string, rate float64) error {
	return c.write("count", name, strconv.FormatInt(value, 10), tags)
}

// Histogram implements Client.
func (c *captureClient) Histogram(name string, value float64, tags []string, rate float64) error {
	return c.write("histogram", name, formatFloat(value), tags)
}

// Timing implements Client.
func (c *captureClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	return c.write("timing", name, strconv.FormatInt(int64(value), 10), tags)
}

// Flush closes the file. It should be called only once at the end of the program.
func (c *captureClient) Flush() error {
	log.Infof("Successfully wrote %d metrics to %q", c.lines, c.f.Name())
	return c.f.Close()
}

var (
	sep   = []byte("|")
	comma = []byte(",")
	nl    = []byte("\n")
)

func (c *captureClient) write(typ, name, value string, tags []string) error {
	c.mu.Lock()
	c.f.WriteString(typ)
	c.f.Write(sep)
	c.f.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
	c.f.Write(sep)
	c.f.WriteString(name)
	c.f.Write(sep)
	c.f.WriteString(value)
	c.f.Write(sep)
	for i, tag := range tags {
		if i > 0 {
			c.f.Write(comma)
		}
		c.f.WriteString(tag)
	}
	c.f.Write(nl)
	c.mu.Unlock()
	atomic.AddUint64(&c.lines, 1)
	return nil
}

func formatFloat(value float64) string {
	v := strconv.FormatFloat(value, 'f', 4, 64)
	if v == "NaN" {
		return "0"
	}
	return v
}
