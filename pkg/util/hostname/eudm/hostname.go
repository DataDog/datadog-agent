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

func formatHostname(name, serial string) (string, error) {
	serial = normalize(serial)
	switch serial {
	case "", "unknown", "none", "na", "n-a", "not-applicable", "not-available", "not-specified", "default-string", "system-serial-number", "to-be-filled-by-o-e-m", "to-be-filled-by-oem":
		return "", errors.New("device serial number is missing or a placeholder")
	}
	if strings.Trim(serial, "0-") == "" || strings.Trim(serial, "f-") == "" {
		return "", errors.New("device serial number is a placeholder")
	}
	name = normalize(name)
	if name == "" {
		return "", errors.New("device name is empty after normalization")
	}
	// Keep the full serial suffix. Do not truncate it into a colliding identity.
	const maxLength = 253
	available := maxLength - len(serial) - 1
	if available < 1 {
		return "", errors.New("device serial number is too long")
	}
	if len(name) > available {
		name = strings.TrimRight(name[:available], "-")
	}
	if name == "" {
		return "", errors.New("device name is empty after truncation")
	}
	hostname := name + "-" + serial
	if err := validate.ValidHostname(hostname); err != nil {
		return "", err
	}
	return hostname, nil
}

// normalize produces a lowercase ASCII name, collapsing separators to hyphens.
func normalize(value string) string {
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
