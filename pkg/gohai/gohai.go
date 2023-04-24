// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package main contains the binary related functions,
// eg. cli parameters
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	// 3p
	log "github.com/cihub/seelog"

	// project
	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/filesystem"
	"github.com/DataDog/gohai/memory"
	"github.com/DataDog/gohai/network"
	"github.com/DataDog/gohai/platform"
	"github.com/DataDog/gohai/processes"
)

// Collector represents a group of information which can be collected
type Collector interface {
	Name() string
	Collect() (interface{}, error)
}

// SelectedCollectors represents a set of collector names
type SelectedCollectors map[string]struct{}

var collectors = []Collector{
	&cpu.Cpu{},
	&filesystem.FileSystem{},
	&memory.Memory{},
	&network.Network{},
	&platform.Platform{},
	&processes.Processes{},
}

var options struct {
	only     SelectedCollectors
	exclude  SelectedCollectors
	logLevel string
	version  bool
}

// version information filled in at build time
var (
	buildDate string
	gitCommit string
	gitBranch string
	goVersion string
)

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

	result["gohai"] = versionMap()

	return
}

func versionMap() (result map[string]interface{}) {
	result = make(map[string]interface{})

	result["git_hash"] = gitCommit
	result["git_branch"] = gitBranch
	result["build_date"] = buildDate
	result["go_version"] = goVersion

	return
}

func versionString() string {
	var buf bytes.Buffer

	if gitCommit != "" {
		fmt.Fprintf(&buf, "Git hash: %s\n", gitCommit)
	}
	if gitBranch != "" {
		fmt.Fprintf(&buf, "Git branch: %s\n", gitBranch)
	}
	if buildDate != "" {
		fmt.Fprintf(&buf, "Build date: %s\n", buildDate)
	}
	if goVersion != "" {
		fmt.Fprintf(&buf, "Go Version: %s\n", goVersion)
	}

	return buf.String()
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

	flag.BoolVar(&options.version, "version", false, "Show version information and exit")
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

	if options.version {
		fmt.Printf("%s", versionString())
		os.Exit(0)
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
