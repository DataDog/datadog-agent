// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

// TestMain is the entry points for functional tests
func TestMain(m *testing.M) {
	flag.Parse()

	if flakeMatrixConfigFile != "" && len(runnerTags) != 0 {
		f, err := os.Open(flakeMatrixConfigFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open flake matrix config file: %s\n", err)
			os.Exit(1)
		}

		matrix := flakeMatrixConfig{}
		if err := yaml.NewDecoder(f).Decode(&matrix); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to decode flake matrix config file: %s\n", err)
			os.Exit(1)
		}

		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close flake matrix config file: %s\n", err)
			os.Exit(1)
		}

		slices.Sort(runnerTags)
		matchCount := 0
		var matchingRunnerConfig runnerConfig
		for _, runner := range matrix.Runners {
			tagsSlice := strings.Split(runner.Tags, ",")
			slices.Sort(tagsSlice)
			if !slices.Equal(runnerTags, tagsSlice) {
				continue
			}
			matchCount++
			if matchCount > 1 {
				fmt.Fprintf(os.Stderr, "Multiple runners matched the given tags: %s\n", runnerTags)
				os.Exit(1)
			}
			matchingRunnerConfig = runner
		}

		if matchCount == 0 {
			fmt.Fprintf(os.Stderr, "No runner matched the given tags: %s\n", runnerTags)
			os.Exit(1)
		}

		var combinedRegex strings.Builder

		for i, flakyTestRegex := range matchingRunnerConfig.FlakyTestsRegex {
			if i > 0 {
				combinedRegex.WriteString("|")
			}
			combinedRegex.WriteString("(")
			combinedRegex.WriteString(flakyTestRegex)
			combinedRegex.WriteString(")")
		}

		if combinedRegex.Len() > 0 {
			flakyTestsRegex, err = regexp.Compile(combinedRegex.String())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to compile combined flaky tests regex for runner with tags %s: %s\n", runnerTags, err)
				os.Exit(1)
			}
			fmt.Printf("Combined flaky tests regex: %s\n", combinedRegex.String())
		}
	}

	preTestsHook()
	retCode := m.Run()
	postTestsHook()

	if commonCfgDir != "" {
		_ = os.RemoveAll(commonCfgDir)
	}

	os.Exit(retCode)
}

var (
	commonCfgDir string

	logLevelStr           string
	logPatterns           stringSlice
	logTags               stringSlice
	ebpfLessEnabled       bool
	flakeMatrixConfigFile string
	runnerTags            stringSlice
	flakyTestsRegex       *regexp.Regexp
)

type runnerConfig struct {
	Tags            string   `yaml:"tags,omitempty"`
	FlakyTestsRegex []string `yaml:"flaky_tests_regex,omitempty"`
}

type flakeMatrixConfig struct {
	Runners []runnerConfig `yaml:"runners,omitempty"`
}

func init() {
	flag.StringVar(&logLevelStr, "loglevel", log.WarnStr, "log level")
	flag.Var(&logPatterns, "logpattern", "List of log pattern")
	flag.Var(&logTags, "logtag", "List of log tag")

	flag.StringVar(&flakeMatrixConfigFile, "flake-matrix", "", "Flake matrix")
	flag.Var(&runnerTags, "runner-tag", "List of runner tags")

	rand.Seed(time.Now().UnixNano())
}

func CheckFlakyTest(t testing.TB) {
	t.Helper()

	if flakyTestsRegex == nil {
		return
	}

	if flakyTestsRegex.MatchString(t.Name()) {
		flake.Mark(t)
	}
}
