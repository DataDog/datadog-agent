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
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

var GitAncestorOnMain = "main"

// TestMain is the entry points for functional tests
func TestMain(m *testing.M) {
	flag.Parse()

	fmt.Printf("Using git ref %s as common ancestor between HEAD and main branch\n", GitAncestorOnMain)

	if shouldCheckForRequiredTests() {
		f, err := os.Open(requiredTestsConfigFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open required tests config file: %s\n", err)
			os.Exit(1)
		}

		var config requiredTestsConfig
		if err := yaml.NewDecoder(f).Decode(&config); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to decode required tests config file: %s\n", err)
			os.Exit(1)
		}

		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close required tests config file: %s\n", err)
			os.Exit(1)
		}

		var requiredTestsPatterns []string
		for _, runnerConfig := range config.Runners {
			// add test patterns for which tags are a subset of the test runner tags
			tagsSlice := strings.Split(runnerConfig.Tags, ",")
			match := true                                   // if no tags -> the runner config matches
			if len(tagsSlice) != 1 || tagsSlice[0] != "*" { // if there is only one tag and it is * -> the runner config matches
				for _, tag := range tagsSlice {
					if !slices.Contains(testRunnerTags, tag) {
						match = false
						break
					}
				}
			}
			if match {
				requiredTestsPatterns = append(requiredTestsPatterns, runnerConfig.RequiredTests...)
			}
		}

		if len(requiredTestsPatterns) > 0 {
			requiredTestsRegex, err = regexp.Compile("(" + strings.Join(requiredTestsPatterns, ")|(") + ")")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to compile required tests regex: %s\nRequired tests patterns: %s\n", err, strings.Join(requiredTestsPatterns, ","))
				os.Exit(1)
			}
			fmt.Printf("Required tests regex: %s\n", requiredTestsRegex.String())
		} else {
			fmt.Printf("No required tests found for the given test runner tags: %s\n", testRunnerTags)
		}
	} else {
		fmt.Printf("No required tests configuration file provided, skipping required tests checks\n")
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
	commonCfgDir       string
	ebpfLessEnabled    bool
	requiredTestsRegex *regexp.Regexp

	logLevelStr             string
	logPatterns             stringSlice
	logTags                 stringSlice
	requiredTestsConfigFile string
	testRunnerTags          stringSlice
)

type testRunnerConfig struct {
	Tags          string   `yaml:"tags,omitempty"`
	RequiredTests []string `yaml:"required_tests,omitempty"`
}

type requiredTestsConfig struct {
	Runners []testRunnerConfig `yaml:"runners,omitempty"`
}

func init() {
	flag.StringVar(&logLevelStr, "loglevel", log.WarnStr, "log level")
	flag.Var(&logPatterns, "logpattern", "List of log pattern")
	flag.Var(&logTags, "logtag", "List of log tag")
	flag.StringVar(&requiredTestsConfigFile, "required-tests-cfg", "", "Configuration for required tests")
	flag.Var(&testRunnerTags, "test-runner-tags", "List of test runner tags")
}

func shouldCheckForRequiredTests() bool {
	return requiredTestsConfigFile != "" && len(testRunnerTags) != 0
}

func CheckRequiredTest(t testing.TB) {
	t.Helper()

	if !shouldCheckForRequiredTests() {
		return
	}

	if requiredTestsRegex == nil || !requiredTestsRegex.MatchString(t.Name()) {
		flake.Mark(t)
	}
}
