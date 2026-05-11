// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

func netstatTCPExtCounters() (map[string]int64, error) {
	f, err := os.Open("/proc/net/netstat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	counters := map[string]int64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		i := strings.IndexRune(line, ':')
		if i == -1 {
			return nil, errors.New("/proc/net/netstat is not fomatted correctly, expected ':'")
		}
		proto := strings.ToLower(line[:i])
		if proto != "tcpext" {
			continue
		}

		counterNames := strings.Split(line[i+2:], " ")

		if !scanner.Scan() {
			return nil, errors.New("/proc/net/netstat is not fomatted correctly, not data line")
		}
		line = scanner.Text()

		counterValues := strings.Split(line[i+2:], " ")
		if len(counterNames) != len(counterValues) {
			return nil, errors.New("/proc/net/netstat is not fomatted correctly, expected same number of columns")
		}

		for j := range counterNames {
			value, err := strconv.ParseInt(counterValues[j], 10, 64)
			if err != nil {
				return nil, err
			}
			counters[counterNames[j]] = value
		}
	}

	return counters, nil
}
