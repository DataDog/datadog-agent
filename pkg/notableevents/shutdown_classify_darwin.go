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

const (
	// maxShutdownTokens bounds the token union. The PMU dictionary on the
	// measured hardware holds 80 entries, but the property is
	// machine-controlled and crosses into an event payload.
	maxShutdownTokens = 128
	// maxShutdownTokenBytes bounds one token.
	maxShutdownTokenBytes = 64
)

// shutdownClassPrecedence orders classifications from most to least
// significant. A collapsing machine drags reset, crash and power tokens along
// with the thermal one, so thermal has to win or the event that matters most
// gets titled as something else. Adding a class here is what makes it
// reachable, so a new class cannot be introduced without being ranked.
var shutdownClassPrecedence = []shutdownClass{
	shutdownClassThermal,
	shutdownClassPower,
	shutdownClassCrash,
	shutdownClassWatchdog,
	shutdownClassHardware,
}

// shutdownTitles and shutdownMessages are fixed per classification rather than
// built from token text, which keeps them inside MaxEventStringBytes
// unconditionally and keeps token data in the custom payload.
//
// The crash wording says "followed a CRASH signal" deliberately: the token
// records a hardware line assertion, and a user holding Control-Command-Power
// asserts it, so claiming a kernel panic would be wrong on the common path.
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

// shutdownFaultFamilies maps a PMU token family to its fault classification.
// Families absent from this map are benign, so an uncatalogued token stays
// silent rather than reporting a fault on every boot of unfamiliar hardware.
//
// The map is deliberately static. The PMU's info-fault_name dictionary
// describes what the hardware can name, not what constitutes a fault, so it
// cannot be used to derive this at runtime.
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

// shutdownBenignTokens lists tokens whose family classifies as a fault but
// which the hardware names for a benign or user-driven condition. Family
// granularity is coarser than the dictionary, so these are the cases where
// inheriting the family's class would report a fault that did not happen.
//
// Excluding a token only changes the outcome when it is the sole evidence for
// its class: classification unions every token, so a machine that genuinely
// browns out still classifies on its other power tokens. That is what keeps
// this list from turning false positives into false negatives.
//
// Entries need a reason. A token that merely looks benign is not enough:
// vddio,vddio_1v2_sgpio0_ok reads like a rail-OK status but appears in a
// dictionary of fault names, where it means the rail-OK signal dropped.
var shutdownBenignTokens = map[string]struct{}{
	// Booting after the battery was charged from depleted, not a regulator
	// failure. The other buck_ tokens do name real faults.
	"buck_boot_charge": {},
	// The power-button double-click window, which is a user interaction rather
	// than a watchdog expiring.
	"timeout,dblclick_timeout": {},
}

// shutdownUnderscoreFamilies lists families whose tokens separate the family
// from the detail with an underscore instead of a comma. Without them
// buck_en_err and pgood_error_idx0 would each resolve to their own family and
// be treated as benign.
//
// Comma separation is still tried first, which is what keeps otp_crc out of the
// thermal ot family.
var shutdownUnderscoreFamilies = []string{"buck", "pgood", "target_off"}

// shutdownCauseResult is one classified boot fault payload.
type shutdownCauseResult struct {
	// Class is the winning classification, chosen by precedence.
	Class shutdownClass
	// PrimaryFamily is the lexicographically first family of Class present.
	// Arbitrary but deterministic, which is what a stable title needs.
	PrimaryFamily string
	// Tokens is the sorted, deduplicated union across every publishing PMU.
	Tokens []string
	// Families is the sorted set of families present, benign ones included.
	Families []string
	// FaultTokens is the subset of Tokens belonging to a fault family.
	FaultTokens []string
	// PMUCount is the number of services that published the property.
	PMUCount int
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

// validatePMUBootFaultInfo rejects a payload that cannot be trusted as event
// content. A rejected read is never partially classified.
func validatePMUBootFaultInfo(info pmuBootFaultInfo) error {
	total := 0
	for _, group := range info.Groups {
		for _, token := range group {
			total++
			if total > maxShutdownTokens {
				return fmt.Errorf("more than %d PMU fault tokens", maxShutdownTokens)
			}
			if len(token) > maxShutdownTokenBytes {
				return fmt.Errorf("PMU fault token exceeds %d bytes", maxShutdownTokenBytes)
			}
			if !isShutdownToken(token) {
				return fmt.Errorf("malformed PMU fault token %q", token)
			}
		}
	}
	return nil
}

// isShutdownToken reports whether a token holds only the characters the PMU
// dictionary uses.
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

// classifyShutdownTokens unions and classifies one boot's tokens. The boolean
// is false when nothing qualifies as a fault, which is the clean-shutdown case
// and must produce no event at all.
func classifyShutdownTokens(info pmuBootFaultInfo) (shutdownCauseResult, bool) {
	tokens := make(map[string]struct{})
	for _, group := range info.Groups {
		for _, token := range group {
			tokens[token] = struct{}{}
		}
	}
	if len(tokens) == 0 {
		return shutdownCauseResult{}, false
	}

	result := shutdownCauseResult{
		Tokens:      make([]string, 0, len(tokens)),
		Families:    make([]string, 0, len(tokens)),
		FaultTokens: make([]string, 0, len(tokens)),
		PMUCount:    len(info.Groups),
	}
	families := make(map[string]struct{})
	classFamilies := make(map[shutdownClass]map[string]struct{})
	for token := range tokens {
		result.Tokens = append(result.Tokens, token)

		family := tokenFamily(token)
		families[family] = struct{}{}

		// Checked before the family lookup, and after Families is recorded: a
		// benign token is still part of the payload, it just cannot elect a
		// class or land in FaultTokens.
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

// lexicographicallyFirst returns the smallest key of a non-empty set.
func lexicographicallyFirst(values map[string]struct{}) string {
	first := ""
	for value := range values {
		if first == "" || value < first {
			first = value
		}
	}
	return first
}
