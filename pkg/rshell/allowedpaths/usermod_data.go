// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"errors"
	"strconv"
	"strings"
)

const maxAccountFileBytes = 1 << 20

// accountRecords rejects unsupported layouts without normalizing unrelated bytes.
func accountRecords(data []byte, fields int) ([][]string, error) {
	if len(data) == 0 || len(data) > maxAccountFileBytes || strings.ContainsAny(string(data), "\x00\r") {
		return nil, errors.New("unsupported account file contents")
	}
	for _, b := range data {
		if (b < 0x20 && b != '\n') || b == 0x7f {
			return nil, errors.New("control byte in account record")
		}
	}
	var records [][]string
	for _, line := range strings.Split(string(data), "\n") {
		if len(line) > 64<<10 {
			return nil, errors.New("account record exceeds size limit")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		record := strings.Split(line, ":")
		if len(record) != fields || record[0] == "" {
			return nil, errors.New("malformed account record")
		}
		records = append(records, record)
	}
	return records, nil
}

func validAccountID(value string) bool {
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return false
	}
	id, err := strconv.ParseUint(value, 10, 32)
	return err == nil && id > 0 && id < 1<<32-1
}

func validateDDAgent(data []byte) error {
	records, err := accountRecords(data, 7)
	if err != nil {
		return err
	}
	found := 0
	for _, record := range records {
		if record[0] == "dd-agent" {
			found++
			if !validAccountID(record[2]) || !validAccountID(record[3]) {
				return errors.New("dd-agent must have non-root local UID and GID")
			}
		}
	}
	if found != 1 {
		return errors.New("expected exactly one local dd-agent user")
	}
	return nil
}

func appendDockerMember(data []byte, shadow bool) ([]byte, error) {
	records, err := accountRecords(data, 4)
	if err != nil {
		return nil, err
	}
	var target []string
	for _, record := range records {
		if record[0] == "docker" {
			if target != nil {
				return nil, errors.New("duplicate docker group")
			}
			target = record
		}
	}
	if target == nil {
		return nil, errors.New("docker group is missing")
	}
	if !shadow && !validAccountID(target[2]) {
		return nil, errors.New("docker must have a non-root local GID")
	}
	member := false
	if target[3] != "" {
		for _, name := range strings.Split(target[3], ",") {
			if name == "" || strings.ContainsAny(name, " \t") {
				return nil, errors.New("malformed docker member list")
			}
			member = member || name == "dd-agent"
		}
	}
	if member {
		return data, nil
	}
	old := strings.Join(target, ":")
	if target[3] != "" {
		target[3] += ","
	}
	target[3] += "dd-agent"
	// Prefixing a newline prevents matching a name embedded in another record.
	updated := strings.Replace("\n"+string(data), "\n"+old, "\n"+strings.Join(target, ":"), 1)[1:]
	if len(updated) > maxAccountFileBytes {
		return nil, errors.New("updated account file exceeds size limit")
	}
	return []byte(updated), nil
}
