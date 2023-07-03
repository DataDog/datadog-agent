// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
)

const CIVisibility = "/ci-visibility"

func init() {
	color.NoColor = false
}

func printHeader(str string) {
	magentaString := color.New(color.FgMagenta, color.Bold).Add(color.Underline)
	fmt.Println()
	magentaString.Println(str)
}

func main() {
	var matches []string
	err := filepath.WalkDir(CIVisibility, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		present, err := regexp.Match("testjson-*", []byte(d.Name()))
		if err != nil {
			return fmt.Errorf("directory regex match: %s", err)
		}

		if !present {
			return nil
		}

		matches = append(matches, path)

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(matches)

	for i, testjson := range matches {
		printHeader(fmt.Sprintf("Reviewing attempt %d", i+1))
		failedTests, err := reviewTests(filepath.Join(testjson, "out.json"))
		if err != nil {
			log.Fatal(err)
		}
		if len(failedTests) > 0 {
			fmt.Println(color.RedString(failedTests))
		} else {
			fmt.Println(color.GreenString(fmt.Sprintf("All tests cleared in attempt %d", i+1)))
			return
		}
	}

	// We want to make sure the exit code is correctly set to
	// failed here, so that the CI job also fails.
	os.Exit(1)
}

type testEvent struct {
	Time    time.Time // encodes as an RFC3339-format string
	Action  string
	Package string
	Test    string
	Elapsed float64 // seconds
	Output  string
}

func reviewTests(jsonFile string) (string, error) {
	var failedTests strings.Builder
	jf, err := os.Open(jsonFile)
	if err != nil {
		return "", fmt.Errorf("open %s: %s", jsonFile, err)
	}

	scanner := bufio.NewScanner(jf)
	for scanner.Scan() {
		var ev testEvent
		data := scanner.Bytes()
		if err := json.Unmarshal(data, &ev); err != nil {
			return "", fmt.Errorf("json unmarshal `%s`: %s", string(data), err)
		}
		if ev.Action == "fail" && ev.Test != "" {
			failedTests.WriteString(fmt.Sprintf("FAIL: %s %s\n", ev.Package, ev.Test))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("json line scan: %s", err)
	}
	return failedTests.String(), nil
}
