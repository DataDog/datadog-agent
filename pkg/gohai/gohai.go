// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package main contains the binary related functions,
// eg. cli parameters
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	// 3p
	log "github.com/cihub/seelog"

	// project
	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/gohai/filesystem"
	"github.com/DataDog/datadog-agent/pkg/gohai/memory"
	"github.com/DataDog/datadog-agent/pkg/gohai/network"
	"github.com/DataDog/datadog-agent/pkg/gohai/platform"
	"github.com/DataDog/datadog-agent/pkg/gohai/processes"
	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

// Collector represents a group of information which can be collected
type Collector interface {
	Name() string
	Collect() (interface{}, error)
}

// CollectorV2 is a compatibility layer between the old 'Collector' interface and
// the way the new API is defined
type CollectorV2[T utils.Jsonable] struct {
	name    string
	collect func() (T, error)
}

// Name returns the name of the CollectorV2
func (collector *CollectorV2[T]) Name() string {
	return collector.name
}

// Collect calls the CollectorV2's collect method and returns only its marshallable object and error
func (collector *CollectorV2[T]) Collect() (interface{}, error) {
	info, err := collector.collect()
	if err != nil {
		return nil, err
	}
	json, _, err := info.AsJSON()
	return json, err
}

// SelectedCollectors represents a set of collector names
type SelectedCollectors map[string]struct{}

var collectors = []Collector{
	&CollectorV2[*cpu.Info]{
		name: "cpu",
		collect: func() (*cpu.Info, error) {
			return cpu.CollectInfo(), nil
		},
	},
	&filesystem.FileSystem{},
	&CollectorV2[*memory.Info]{
		name: "memory",
		collect: func() (*memory.Info, error) {
			return memory.CollectInfo(), nil
		},
	},
	&CollectorV2[*network.Info]{
		name:    "network",
		collect: network.CollectInfo,
	},
	&CollectorV2[*platform.Info]{
		name: "platform",
		collect: func() (*platform.Info, error) {
			return platform.CollectInfo(), nil
		},
	},
	&processes.Processes{},
}

var options struct {
	only     SelectedCollectors
	exclude  SelectedCollectors
	logLevel string
}

// Collect fills the result map with the collector information under their name key
func Collect() (result map[string]interface{}, err error) {
	result = make(map[string]interface{})

	for _, collector := range collectors {
		if shouldCollect(collector) {
			c, err := collector.Collect()
			if err != nil {
				log.Warnf("[%s] %s", collector.Name(), err)
			}
			if c != nil {
				result[collector.Name()] = c
			}
		}
	}

	return
}

// Implement the flag.Value interface
func (sc *SelectedCollectors) String() string {
	collectorSlice := make([]string, 0, len(*sc))
	for collectorName := range *sc {
		collectorSlice = append(collectorSlice, collectorName)
	}
	sort.Strings(collectorSlice)
	return fmt.Sprint(collectorSlice)
}

// Set adds the given comma-separated list of collector names to the selected set.
func (sc *SelectedCollectors) Set(value string) error {
	for _, collectorName := range strings.Split(value, ",") {
		(*sc)[collectorName] = struct{}{}
	}
	return nil
}

// Return whether we should collect on a given collector, depending on the parsed flags
func shouldCollect(collector Collector) bool {
	if _, ok := options.only[collector.Name()]; len(options.only) > 0 && !ok {
		return false
	}

	if _, ok := options.exclude[collector.Name()]; ok {
		return false
	}

	return true
}

// Will be called after all the imported packages' init() have been called
// Define collector-specific flags in their packages' init() function
func init() {
	options.only = make(SelectedCollectors)
	options.exclude = make(SelectedCollectors)

	flag.Var(&options.only, "only", "Run only the listed collectors (comma-separated list of collector names)")
	flag.Var(&options.exclude, "exclude", "Run all the collectors except those listed (comma-separated list of collector names)")
	flag.StringVar(&options.logLevel, "log-level", "info", "Log level (one of 'warn', 'info', 'debug')")
}

func main() {
	defer log.Flush()

	flag.Parse()

	err := initLogging(options.logLevel)
	if err != nil {
		panic(fmt.Sprintf("Unable to initialize logger: %s", err))
	}

	gohai, err := Collect()
	if err != nil {
		panic(err)
	}

	buf, err := json.Marshal(gohai)
	if err != nil {
		panic(err)
	}

	os.Stdout.Write(buf)
}
