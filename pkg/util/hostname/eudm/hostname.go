// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eudm resolves a device hostname without network-derived naming.
package eudm

import (
	"errors"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
)

// formatHostname validates the device name unchanged and optionally appends a
// normalized serial suffix. Neither the name nor the serial is truncated.
func formatHostname(name, serial string, useSerial bool) (string, error) {
	if err := validate.ValidHostname(name); err != nil {
		return "", err
	}

	suffix := ""
	if useSerial {
		serial = normalizeSerial(serial)
		switch serial {
		case "", "unknown", "none", "na", "n-a", "not-applicable", "not-available", "not-specified", "default-string", "system-serial-number", "to-be-filled-by-o-e-m", "to-be-filled-by-oem":
			return "", errors.New("device serial number is missing or a placeholder")
		}
		if strings.Trim(serial, "0-") == "" || strings.Trim(serial, "f-") == "" {
			return "", errors.New("device serial number is a placeholder")
		}
		suffix = "-" + serial
	}
	hostname := name + suffix
	const maxLength = 253
	if len(hostname) > maxLength {
		return "", errors.New("device hostname is too long")
	}

	if err := validate.ValidHostname(hostname); err != nil {
		return "", err
	}
	return hostname, nil
}

// normalizeSerial lowercases ASCII letters and collapses separators to hyphens.
func normalizeSerial(value string) string {
	var b strings.Builder
	separator := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if separator && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	return b.String()
}
