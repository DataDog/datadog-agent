// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ioscmd

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// checkText rejects empty strings and strings containing control characters.
// Every user-supplied string goes through this (or checkToken), since an
// embedded newline would let a value inject arbitrary commands.
func checkText(field, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s must not contain control characters", field)
	}
	return nil
}

// checkToken is like checkText but also rejects whitespace, for values that
// must be a single CLI word (interface names, VRF names, ...).
func checkToken(field, s string) error {
	if s == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return fmt.Errorf("%s must be a single word, got %q", field, s)
	}
	return nil
}

func checkOneOf(field, s string, allowed ...string) error {
	if !slices.Contains(allowed, s) {
		return fmt.Errorf("%s must be one of %q, got %q", field, allowed, s)
	}
	return nil
}

func checkIPv4(field, s string) error {
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is4() {
		return fmt.Errorf("%s must be an IPv4 address, got %q", field, s)
	}
	return nil
}

func checkIPv4Mask(field, s string) error {
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is4() {
		return fmt.Errorf("%s must be a dotted-quad mask, got %q", field, s)
	}
	b := addr.As4()
	if _, bits := net.IPMask(b[:]).Size(); bits == 0 {
		return fmt.Errorf("%s must be a contiguous mask, got %q", field, s)
	}
	return nil
}

func checkVlan(field string, id int) error {
	if id < 1 || id > 4094 {
		return fmt.Errorf("%s must be between 1 and 4094, got %d", field, id)
	}
	return nil
}

var vlanListRe = regexp.MustCompile(`^\d+(-\d+)?(,\d+(-\d+)?)*$`)

func checkVlanList(field, s string) error {
	if !vlanListRe.MatchString(s) {
		return fmt.Errorf("%s must be a VLAN list like \"10,20,30-40\", got %q", field, s)
	}
	return nil
}
