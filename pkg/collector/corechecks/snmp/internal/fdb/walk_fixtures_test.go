// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdb

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
)

func loadWalkFile(path string) (*session.FakeSession, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sess := session.CreateFakeSession()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		oid, value, err := parseWalkLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", line, err)
		}
		sess.SetInt(oid, value)
	}
	return sess, scanner.Err()
}

func parseWalkLine(line string) (string, int, error) {
	if strings.Contains(line, "|") {
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			return "", 0, fmt.Errorf("invalid snmprec line")
		}
		oid := strings.TrimLeft(parts[0], ".")
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			return "", 0, err
		}
		return oid, n, nil
	}

	oid, rest, ok := strings.Cut(line, " = ")
	if !ok {
		return "", 0, fmt.Errorf("invalid snmpwalk line")
	}
	oid = strings.TrimLeft(oid, ".")
	_, val, ok := strings.Cut(rest, ": ")
	if !ok {
		return "", 0, fmt.Errorf("missing typed value")
	}
	n, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		return "", 0, err
	}
	return oid, n, nil
}
