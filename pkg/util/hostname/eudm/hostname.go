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

// formatHostname returns the device name with an optional normalized serial suffix.
// With preserveName, the name is validated unchanged; otherwise it is normalized
// and may be truncated to keep the result within 253 characters.
func formatHostname(name, serial string, useSerial, preserveName bool) (string, error) {
	if preserveName {
		if err := validate.ValidHostname(name); err != nil {
			return "", err
		}
	} else {
		name = normalize(name)
	}

	suffix := ""
	if useSerial {
		serial = normalize(strings.ToLower(serial))
		switch serial {
		case "", "unknown", "none", "na", "n-a", "not-applicable", "not-available", "not-specified", "default-string", "system-serial-number", "to-be-filled-by-o-e-m", "to-be-filled-by-oem":
			return "", errors.New("device serial number is missing or a placeholder")
		}
		if strings.Trim(serial, "0-") == "" || strings.Trim(serial, "f-") == "" {
			return "", errors.New("device serial number is a placeholder")
		}
		suffix = "-" + serial
	}
	if name == "" {
		return "", errors.New("device name is empty after normalization")
	}
	// Keep the full serial suffix. Do not truncate it into a colliding identity.
	const maxLength = 253
	available := maxLength - len(suffix)
	if available < 1 {
		return "", errors.New("device serial number is too long")
	}
	if len(name) > available {
		if preserveName {
			return "", errors.New("device hostname is too long")
		}
		name = strings.TrimRight(name[:available], "-")
	}
	if name == "" {
		return "", errors.New("device name is empty after truncation")
	}
	hostname := name + suffix
	if err := validate.ValidHostname(hostname); err != nil {
		return "", err
	}
	return hostname, nil
}

// normalize preserves ASCII letter casing and collapses separators to hyphens.
func normalize(value string) string {
	var b strings.Builder
	separator := false
	for _, r := range strings.TrimSpace(value) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
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
