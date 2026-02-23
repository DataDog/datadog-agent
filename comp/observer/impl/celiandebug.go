// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This is used to simulate sending internal metrics to Datadog.
// This will periodically write metrics as a json payload and send them via a log.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

type CelianDebug struct {
	interval  time.Duration
	mu        sync.Mutex
	metrics   map[string]float64
	callbacks []func(map[string]float64)
	logPath   string
}

var celiandebug *CelianDebug

func NewCelianDebug(interval time.Duration) *CelianDebug {
	return &CelianDebug{
		interval: interval,
		metrics:  make(map[string]float64),
		logPath:  "/tmp/celiandebug.log",
	}
}

func (c *CelianDebug) Run() {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.runCallbacks()
				c.sendMetrics()
			}
		}
	}()
}

func (c *CelianDebug) runCallbacks() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, callback := range c.callbacks {
		callback(c.metrics)
	}
}

func (c *CelianDebug) sendMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Send metrics
	c.metrics["running"] = 1
	data, err := json.Marshal(c.metrics)
	if err != nil {
		log.Errorf("Failed to marshal metrics: %v", err)
		return
	}
	fmt.Printf("Sending metrics: %s\n", string(data))

	// Append to the log file
	file, err := os.OpenFile(c.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Errorf("Failed to open log file: %v", err)
		return
	}
	defer file.Close()
	_, err = file.WriteString(string(data) + "\n")
	if err != nil {
		log.Errorf("Failed to write metrics to file: %v", err)
		return
	}

	c.metrics = make(map[string]float64)
}

func init() {
	celiandebug = NewCelianDebug(10 * time.Second)
	celiandebug.Run()
}
