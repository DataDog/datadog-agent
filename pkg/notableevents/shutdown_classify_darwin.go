// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"fmt"
	"sort"
	"strings"
)

// shutdownClass is the fault classification of a previous shutdown.
type shutdownClass string

const (
	shutdownClassNone     shutdownClass = ""
	shutdownClassThermal  shutdownClass = "thermal"
	shutdownClassPower    shutdownClass = "power"
	shutdownClassCrash    shutdownClass = "crash"
	shutdownClassWatchdog shutdownClass = "watchdog"
	shutdownClassHardware shutdownClass = "hardware"
)

var shutdownClassPrecedence = []shutdownClass{
	shutdownClassThermal,
	shutdownClassPower,
	shutdownClassCrash,
	shutdownClassWatchdog,
	shutdownClassHardware,
}

var shutdownTitles = map[shutdownClass]string{
	shutdownClassThermal:  "macOS overheated shutdown",
	shutdownClassPower:    "macOS power fault shutdown",
	shutdownClassCrash:    "macOS crash shutdown",
	shutdownClassWatchdog: "macOS watchdog timeout shutdown",
	shutdownClassHardware: "macOS hardware fault shutdown",
}

var shutdownMessages = map[shutdownClass]string{
	shutdownClassThermal:  "The previous shutdown was caused by a thermal fault",
	shutdownClassPower:    "The previous shutdown was caused by a power fault",
	shutdownClassCrash:    "The previous shutdown followed a CRASH signal from the processor",
	shutdownClassWatchdog: "The previous shutdown was caused by a watchdog timeout",
	shutdownClassHardware: "The previous shutdown was caused by a hardware fault",
}

var shutdownFaultFamilies = map[string]shutdownClass{
	// Thermal.
	"ot":       shutdownClassThermal, // over-temperature, 12 tokens
	"sochot":   shutdownClassThermal, // SoC-hot pin assertion
	"ntc_shdn": shutdownClassThermal, // thermistor shutdown

	// Power.
	"uv":             shutdownClassPower, // under-voltage / UVLO
	"ov":             shutdownClassPower, // over-voltage / OVLO
	"oc":             shutdownClassPower, // over-current
	"buck":           shutdownClassPower, // buck-converter regulation failure
	"pgood":          shutdownClassPower, // power-good rail assertion failure
	"vddio":          shutdownClassPower, // I/O rail not OK
	"ldo_dig_ovs":    shutdownClassPower, // digital LDO over-voltage
	"cp_wdog_expiry": shutdownClassPower, // charge-pump watchdog expiry
	"emerg_shdn":     shutdownClassPower, // emergency shutdown

	// Watchdog.
	"timeout": shutdownClassWatchdog, // firmware and watchdog timeouts

	// Crash. Covers crash0_in-crash2_in, crash_in, hyp_fw_crash and
	// hyp_hw_crash. Known to fire from a user force restart, which is accepted:
	// the alternative dropped genuine AP and hypervisor crashes silently.
	"crash": shutdownClassCrash,

	// Hardware.
	"spmi":    shutdownClassHardware, // SoC-to-PMU bus fault
	"sgpio":   shutdownClassHardware, // serial GPIO fault
	"fault":   shutdownClassHardware, // generic FAULT input asserted
	"otp_crc": shutdownClassHardware, // OTP trim-memory CRC failure
}

var shutdownBenignTokens = map[string]struct{}{
	// Booting after the battery was charged from depleted, not a regulator
	// failure. The other buck_ tokens do name real faults.
	"buck_boot_charge": {},
	// The power-button double-click window, which is a user interaction rather
	// than a watchdog expiring.
	"timeout,dblclick_timeout": {},
}

var shutdownUnderscoreFamilies = []string{"buck", "pgood", "target_off"}

type shutdownCauseResult struct {
	Class shutdownClass
	// PrimaryFamily is the lexicographically first family of Class present.
	// Arbitrary but deterministic, which is what a stable title needs.
	PrimaryFamily string
	Tokens        []string
	Families      []string
	FaultTokens   []string
}

// tokenFamily returns the family a PMU fault token belongs to: the prefix
// before the first comma, a declared underscore-separated family, or the whole
// token.
func tokenFamily(token string) string {
	if family, _, found := strings.Cut(token, ","); found {
		return family
	}
	longest := ""
	for _, family := range shutdownUnderscoreFamilies {
		if strings.HasPrefix(token, family+"_") && len(family) > len(longest) {
			longest = family
		}
	}
	if longest != "" {
		return longest
	}
	return token
}

// validatePMUBootFaultInfo rejects a payload with a malformed token or more
// distinct tokens than the known dictionary. A token already truncated by the
// native reader to the shared size bound is indistinguishable from a
// full-length token and is classified as-is; truncation-detection is
// intentionally not this function's job. The token bound is charged per
// distinct token, not per slice entry: the real IOKit-backed reader already
// dedups, but this function accepts any pmuBootFaultInfo, including
// hand-constructed ones with duplicates.
func validatePMUBootFaultInfo(info pmuBootFaultInfo) error {
	distinct := make(map[string]struct{})
	for _, token := range info.Tokens {
		if len(token) > maxShutdownTokenBytes {
			return fmt.Errorf("PMU fault token exceeds %d bytes", maxShutdownTokenBytes)
		}
		if !isShutdownToken(token) {
			return fmt.Errorf("malformed PMU fault token %q", token)
		}
		distinct[token] = struct{}{}
		if len(distinct) > maxShutdownTokens {
			return fmt.Errorf("more than %d distinct PMU fault tokens", maxShutdownTokens)
		}
	}
	return nil
}

func isShutdownToken(token string) bool {
	if token == "" {
		return false
	}
	for _, char := range []byte(token) {
		switch {
		case char >= 'a' && char <= 'z',
			char >= '0' && char <= '9',
			char == '_', char == ',':
		default:
			return false
		}
	}
	return true
}

func classifyShutdownTokens(info pmuBootFaultInfo) (shutdownCauseResult, bool) {
	tokens := make(map[string]struct{}, len(info.Tokens))
	for _, token := range info.Tokens {
		tokens[token] = struct{}{}
	}
	if len(tokens) == 0 {
		return shutdownCauseResult{}, false
	}

	result := shutdownCauseResult{
		Tokens:      make([]string, 0, len(tokens)),
		Families:    make([]string, 0, len(tokens)),
		FaultTokens: make([]string, 0, len(tokens)),
	}
	families := make(map[string]struct{})
	classFamilies := make(map[shutdownClass]map[string]struct{})
	for token := range tokens {
		result.Tokens = append(result.Tokens, token)

		family := tokenFamily(token)
		families[family] = struct{}{}

		if _, benign := shutdownBenignTokens[token]; benign {
			continue
		}

		class, isFault := shutdownFaultFamilies[family]
		if !isFault {
			continue
		}
		result.FaultTokens = append(result.FaultTokens, token)
		if classFamilies[class] == nil {
			classFamilies[class] = make(map[string]struct{})
		}
		classFamilies[class][family] = struct{}{}
	}
	for family := range families {
		result.Families = append(result.Families, family)
	}
	sort.Strings(result.Tokens)
	sort.Strings(result.Families)
	sort.Strings(result.FaultTokens)

	for _, class := range shutdownClassPrecedence {
		present := classFamilies[class]
		if len(present) == 0 {
			continue
		}
		result.Class = class
		result.PrimaryFamily = lexicographicallyFirst(present)
		return result, true
	}
	return shutdownCauseResult{}, false
}

func lexicographicallyFirst(values map[string]struct{}) string {
	first := ""
	for value := range values {
		if first == "" || value < first {
			first = value
		}
	}
	return first
}
