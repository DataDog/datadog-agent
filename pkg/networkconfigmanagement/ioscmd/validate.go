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
)

// Every user-supplied string goes through checkText or one of the allowlist
// checks below before it is rendered. Values are spliced into CLI lines
// verbatim, so anything the device interprets specially must be rejected: a
// newline starts a new command, `|` chains an output modifier (`redirect`,
// `tee`, ...) or a shell escape, and `?` invokes context-sensitive help.

// checkText rejects blank strings, strings containing anything other than
// printable ASCII, and strings containing `|` or `?`. It is for free-form
// values such as descriptions and pipe regexes.
func checkText(field, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.IndexFunc(s, func(r rune) bool { return r < ' ' || r > '~' || r == '|' || r == '?' }) >= 0 {
		return fmt.Errorf("%s must be printable ASCII without '|' or '?', got %q", field, s)
	}
	return nil
}

// checkPattern returns an error unless s is non-empty and matches re.
func checkPattern(field, s string, re *regexp.Regexp, want string) error {
	if s == "" {
		return fmt.Errorf("%s is required", field)
	}
	if !re.MatchString(s) {
		return fmt.Errorf("%s must be %s, got %q", field, want, s)
	}
	return nil
}

// interfaceNameRe matches full or abbreviated interface names, including
// subinterfaces and channelized ports: Gi1/0/1, Port-channel1,
// TenGigabitEthernet1/1/1.100, Serial0/0/0:0.
var interfaceNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z-]*[0-9][0-9/.:]*$`)

func checkInterfaceName(field, s string) error {
	return checkPattern(field, s, interfaceNameRe, `an interface name like "GigabitEthernet1/0/1"`)
}

// hostnameRe matches an RFC 1123 hostname label that starts with a letter.
var hostnameRe = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func checkHostname(field, s string) error {
	return checkPattern(field, s, hostnameRe, "a hostname of letters, digits, and hyphens")
}

// nameRe matches names of configuration objects such as VRFs and ACLs.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func checkName(field, s string) error {
	return checkPattern(field, s, nameRe, "a name of letters, digits, '_', '.', and '-'")
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
