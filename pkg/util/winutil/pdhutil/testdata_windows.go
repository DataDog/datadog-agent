// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package pdhutil

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// CounterStrings houses the mocked version of the registry-based counter strings database
type CounterStrings struct {
	initialized  bool
	filename     string
	data         []string
	counterIndex map[int]string // maps the counter index to the string in the data array
}

// AvailableCounters houses the mocked version of the available counters & instances
type AvailableCounters struct {
	initialized      bool
	filename         string
	countersByClass  map[string][]string
	instancesByClass map[string][]string
}

// ReadCounterStrings reads the counter strings from the provided file
func ReadCounterStrings(fn string) (CounterStrings, error) {
	var cs CounterStrings
	cs.initialized = true
	cs.counterIndex = make(map[int]string)
	cs.filename = fn
	inFile, _ := os.Open(fn)
	defer inFile.Close()
	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		cs.data = append(cs.data, scanner.Text())
	}
	for i := 0; i < len(cs.data)-2; i += 2 {
		ci, _ := strconv.Atoi(cs.data[i])
		cs.counterIndex[ci] = cs.data[i+1]
	}

	return cs, nil
}

// ReadCounters reads the available PDH counters from a static text file
func ReadCounters(fn string) (AvailableCounters, error) {
	var ac AvailableCounters
	ac.initialized = true
	ac.countersByClass = make(map[string][]string)
	ac.instancesByClass = make(map[string][]string)
	ac.filename = fn
	inFile, _ := os.Open(fn)
	defer inFile.Close()

	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)

	tmpInstancesByClass := make(map[string]map[string]bool)
	tmpCountersByClass := make(map[string]map[string]bool)

	for scanner.Scan() {
		scannertext := scanner.Text()
		parts := strings.Split(scannertext, "!")
		clss := strings.TrimSpace(parts[0])
		inst := strings.TrimSpace(parts[1])
		counter := strings.TrimSpace(parts[2])
		if _, ok := tmpCountersByClass[clss]; !ok {
			tmpCountersByClass[clss] = make(map[string]bool)
		}
		tmpCountersByClass[clss][counter] = true

		if len(inst) > 0 {
			if _, ok := tmpInstancesByClass[clss]; !ok {
				tmpInstancesByClass[clss] = make(map[string]bool)
			}
			tmpInstancesByClass[clss][inst] = true
		}
	}
	for clss, instancemap := range tmpInstancesByClass {
		for instance := range instancemap {
			ac.instancesByClass[clss] = append(ac.instancesByClass[clss], instance)
		}
	}
	for clss, countermap := range tmpCountersByClass {
		for ctr := range countermap {
			ac.countersByClass[clss] = append(ac.countersByClass[clss], ctr)
		}
	}
	return ac, nil
}
