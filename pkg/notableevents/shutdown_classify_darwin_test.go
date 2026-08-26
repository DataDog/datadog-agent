// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Measured payloads from the PMU boot-fault test run, deduped to their
// distinct token union. Pattern C is load-bearing: the only real-trigger
// payload that also emits an event.
var (
	measuredPatternACleanShutdown = pmuBootFaultInfo{Tokens: []string{
		"target_off_restart", "wdog,reset_in_1", "rst_in,reset_in_1_deassert",
	}}
	measuredPatternBReboot = pmuBootFaultInfo{Tokens: []string{
		"wdog,reset_in_1", "rst_in,reset_in_1_deassert",
	}}
	measuredPatternCButtonForce = pmuBootFaultInfo{Tokens: []string{
		"rst", "btn_rst,btn_seq_reset", "target_off_restart", "crash,crash0_in", "wdog,reset_in_1", "rst_in,reset_in_1_deassert",
	}}
)

func TestClassifyShutdownTokens(t *testing.T) {
	tests := []struct {
		name          string
		info          pmuBootFaultInfo
		expectEvent   bool
		expectClass   shutdownClass
		expectPrimary string
		expectFaults  []string
	}{
		{
			name: "pattern A clean shutdown",
			info: measuredPatternACleanShutdown,
		},
		{
			name: "pattern B sudo reboot",
			info: measuredPatternBReboot,
		},
		{
			name:          "pattern C button force restart",
			info:          measuredPatternCButtonForce,
			expectEvent:   true,
			expectClass:   shutdownClassCrash,
			expectPrimary: "crash",
			expectFaults:  []string{"crash,crash0_in"},
		},
		{
			name:          "thermal",
			info:          pmuBootFaultInfo{Tokens: []string{"ot,tdie_overtemp"}},
			expectEvent:   true,
			expectClass:   shutdownClassThermal,
			expectPrimary: "ot",
			expectFaults:  []string{"ot,tdie_overtemp"},
		},
		{
			name:          "thermal alternate family",
			info:          pmuBootFaultInfo{Tokens: []string{"sochot,reset_in_3"}},
			expectEvent:   true,
			expectClass:   shutdownClassThermal,
			expectPrimary: "sochot",
			expectFaults:  []string{"sochot,reset_in_3"},
		},
		{
			name:          "thermal singleton",
			info:          pmuBootFaultInfo{Tokens: []string{"ntc_shdn"}},
			expectEvent:   true,
			expectClass:   shutdownClassThermal,
			expectPrimary: "ntc_shdn",
			expectFaults:  []string{"ntc_shdn"},
		},
		{
			name:          "hypervisor crash",
			info:          pmuBootFaultInfo{Tokens: []string{"crash,hyp_fw_crash"}},
			expectEvent:   true,
			expectClass:   shutdownClassCrash,
			expectPrimary: "crash",
			expectFaults:  []string{"crash,hyp_fw_crash"},
		},
		{
			name: "thermal wins over every other class",
			info: pmuBootFaultInfo{Tokens: []string{
				"ot,overtemp", "uv,vddmain_uvlo", "crash,crash_in", "timeout,watchdog_timeout",
			}},
			expectEvent:   true,
			expectClass:   shutdownClassThermal,
			expectPrimary: "ot",
			expectFaults: []string{
				"crash,crash_in", "ot,overtemp", "timeout,watchdog_timeout", "uv,vddmain_uvlo",
			},
		},
		{
			name: "power wins without thermal",
			info: pmuBootFaultInfo{Tokens: []string{
				"uv,vddmain_uvlo", "crash,crash_in", "timeout,watchdog_timeout",
			}},
			expectEvent:   true,
			expectClass:   shutdownClassPower,
			expectPrimary: "uv",
			expectFaults: []string{
				"crash,crash_in", "timeout,watchdog_timeout", "uv,vddmain_uvlo",
			},
		},
		{
			name: "crash wins over watchdog",
			info: pmuBootFaultInfo{Tokens: []string{
				"crash,crash_in", "timeout,watchdog_timeout",
			}},
			expectEvent:   true,
			expectClass:   shutdownClassCrash,
			expectPrimary: "crash",
			expectFaults:  []string{"crash,crash_in", "timeout,watchdog_timeout"},
		},
		{
			name:          "underscore separated power family",
			info:          pmuBootFaultInfo{Tokens: []string{"buck_en_err", "pgood_error_idx0"}},
			expectEvent:   true,
			expectClass:   shutdownClassPower,
			expectPrimary: "buck",
			expectFaults:  []string{"buck_en_err", "pgood_error_idx0"},
		},
		{
			name: "unknown family stays silent",
			info: pmuBootFaultInfo{Tokens: []string{"nonsense,whatever"}},
		},
		{
			name:          "unknown thermal suffix still classifies",
			info:          pmuBootFaultInfo{Tokens: []string{"ot,some_future_sensor"}},
			expectEvent:   true,
			expectClass:   shutdownClassThermal,
			expectPrimary: "ot",
			expectFaults:  []string{"ot,some_future_sensor"},
		},
		{
			name:          "unknown crash suffix still classifies",
			info:          pmuBootFaultInfo{Tokens: []string{"crash,some_future_line"}},
			expectEvent:   true,
			expectClass:   shutdownClassCrash,
			expectPrimary: "crash",
			expectFaults:  []string{"crash,some_future_line"},
		},
		{
			name: "benign payload mixed with a fault",
			info: pmuBootFaultInfo{Tokens: []string{
				"target_off_restart", "wdog,reset_in_1", "rst_in,reset_in_1_deassert", "ot,overtemp",
			}},
			expectEvent:   true,
			expectClass:   shutdownClassThermal,
			expectPrimary: "ot",
			expectFaults:  []string{"ot,overtemp"},
		},
		{
			name: "absent property",
			info: pmuBootFaultInfo{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, validatePMUBootFaultInfo(test.info))

			result, emit := classifyShutdownTokens(test.info)
			require.Equal(t, test.expectEvent, emit)
			if !emit {
				assert.Equal(t, shutdownCauseResult{}, result)
				return
			}

			assert.Equal(t, test.expectClass, result.Class)
			assert.Equal(t, test.expectPrimary, result.PrimaryFamily)
			assert.Equal(t, test.expectFaults, result.FaultTokens)
			assert.NotEmpty(t, shutdownTitles[result.Class])
			assert.NotEmpty(t, shutdownMessages[result.Class])

			// Every token stays in Tokens whether or not it is a fault.
			for _, token := range test.info.Tokens {
				assert.Contains(t, result.Tokens, token)
				assert.Contains(t, result.Families, tokenFamily(token))
			}
			assert.True(t, sortedAscending(result.Tokens))
			assert.True(t, sortedAscending(result.Families))
			assert.True(t, sortedAscending(result.FaultTokens))
		})
	}
}

// TestClassifyShutdownTokensPatternCPayload pins the payload the crash
// classification rests on: exactly one fault token, and every benign token
// retained but excluded from it.
func TestClassifyShutdownTokensPatternCPayload(t *testing.T) {
	result, emit := classifyShutdownTokens(measuredPatternCButtonForce)
	require.True(t, emit)

	assert.Equal(t, []string{"crash,crash0_in"}, result.FaultTokens)
	assert.Equal(t, []string{
		"btn_rst,btn_seq_reset",
		"crash,crash0_in",
		"rst",
		"rst_in,reset_in_1_deassert",
		"target_off_restart",
		"wdog,reset_in_1",
	}, result.Tokens)
	assert.Equal(t, []string{
		"btn_rst", "crash", "rst", "rst_in", "target_off", "wdog",
	}, result.Families)
	assert.Equal(t, "macOS crash shutdown", shutdownTitles[result.Class])
}

// TestClassifyShutdownTokensIgnoresTokenOrdering guards the dedup identity:
// IOKit service enumeration order is not stable across boots, so a permuted
// token list must classify identically.
func TestClassifyShutdownTokensIgnoresTokenOrdering(t *testing.T) {
	permuted := pmuBootFaultInfo{Tokens: []string{
		measuredPatternCButtonForce.Tokens[3],
		measuredPatternCButtonForce.Tokens[0],
		measuredPatternCButtonForce.Tokens[2],
		measuredPatternCButtonForce.Tokens[5],
		measuredPatternCButtonForce.Tokens[1],
		measuredPatternCButtonForce.Tokens[4],
	}}

	expected, emit := classifyShutdownTokens(measuredPatternCButtonForce)
	require.True(t, emit)
	actual, emit := classifyShutdownTokens(permuted)
	require.True(t, emit)

	assert.Equal(t, expected, actual)
}

func TestValidatePMUBootFaultInfoRejectsUntrustedPayloads(t *testing.T) {
	oversized := make([]string, maxShutdownTokens+1)
	for index := range oversized {
		oversized[index] = fmt.Sprintf("ot,sensor_%d", index)
	}

	tests := []struct {
		name string
		info pmuBootFaultInfo
	}{
		{
			name: "too many tokens",
			info: pmuBootFaultInfo{Tokens: oversized},
		},
		{
			name: "oversized token",
			info: pmuBootFaultInfo{Tokens: []string{strings.Repeat("a", maxShutdownTokenBytes+1)}},
		},
		{
			name: "markup in a token",
			info: pmuBootFaultInfo{Tokens: []string{"ot,<script>"}},
		},
		{
			name: "uppercase in a token",
			info: pmuBootFaultInfo{Tokens: []string{"OT,OVERTEMP"}},
		},
		{
			name: "whitespace in a token",
			info: pmuBootFaultInfo{Tokens: []string{"ot, overtemp"}},
		},
		{
			name: "empty token",
			info: pmuBootFaultInfo{Tokens: []string{""}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, validatePMUBootFaultInfo(test.info))
		})
	}
}

// TestValidatePMUBootFaultInfoBoundsTheDistinctUnion proves the bound is
// charged per distinct token, not per occurrence, since a hand-built
// pmuBootFaultInfo could contain duplicates a raw count would wrongly reject.
func TestValidatePMUBootFaultInfoBoundsTheDistinctUnion(t *testing.T) {
	dictionary := allPMUFaultTokens()
	require.Less(t, len(dictionary), maxShutdownTokens)

	var republished pmuBootFaultInfo
	for range 3 {
		republished.Tokens = append(republished.Tokens, dictionary...)
	}
	// 240 raw tokens, 80 distinct.
	require.NoError(t, validatePMUBootFaultInfo(republished))

	// The bound still applies to the union itself.
	oversized := make([]string, maxShutdownTokens+1)
	for index := range oversized {
		oversized[index] = fmt.Sprintf("ot,sensor_%d", index)
	}
	require.Error(t, validatePMUBootFaultInfo(pmuBootFaultInfo{Tokens: oversized}))
}

// TestValidatePMUBootFaultInfoAcceptsLongTokens covers the case the native
// reader used to drop and the classifier used to reject: a long token from
// unfamiliar hardware now classifies by family instead of voiding the payload.
func TestValidatePMUBootFaultInfoAcceptsLongTokens(t *testing.T) {
	token := "ot," + strings.Repeat("a", maxShutdownTokenBytes-3)
	require.Len(t, token, maxShutdownTokenBytes)

	info := pmuBootFaultInfo{Tokens: []string{token}}
	require.NoError(t, validatePMUBootFaultInfo(info))

	result, emit := classifyShutdownTokens(info)
	require.True(t, emit)
	assert.Equal(t, shutdownClassThermal, result.Class)
}

func TestValidatePMUBootFaultInfoAcceptsTheFullDictionary(t *testing.T) {
	for _, token := range allPMUFaultTokens() {
		require.NoError(t, validatePMUBootFaultInfo(pmuBootFaultInfo{Tokens: []string{token}}), token)
	}
}

// TestPMUFaultTokenDictionaryCoverage asserts the documented 59 fault / 21
// benign split over the measured PMU's 80 tokens, catching a typo in
// shutdownFaultFamilies that would silently misclassify. The split follows
// classification, so it also accounts for shutdownBenignTokens, not just the
// family map.
func TestPMUFaultTokenDictionaryCoverage(t *testing.T) {
	tokens := allPMUFaultTokens()
	require.Len(t, tokens, 80)

	faults, benign := 0, 0
	byClass := make(map[shutdownClass]int)
	for _, token := range tokens {
		family := tokenFamily(token)
		require.NotEmpty(t, family, token)

		class, isFault := shutdownFaultFamilies[family]
		if _, excluded := shutdownBenignTokens[token]; excluded {
			isFault = false
		}
		if !isFault {
			benign++
			continue
		}
		faults++
		byClass[class]++
	}

	assert.Equal(t, 59, faults, "fault tokens")
	assert.Equal(t, 21, benign, "benign tokens")
	assert.Equal(t, map[shutdownClass]int{
		shutdownClassThermal:  14,
		shutdownClassPower:    29,
		shutdownClassCrash:    6,
		shutdownClassWatchdog: 5,
		shutdownClassHardware: 5,
	}, byClass)
}

// TestShutdownBenignTokensAreOtherwiseFaults guards against dead weight: an
// entry whose family isn't a fault family, or one absent from the dictionary,
// signals a typo or untested hardware.
func TestShutdownBenignTokensAreOtherwiseFaults(t *testing.T) {
	dictionary := make(map[string]struct{})
	for _, token := range allPMUFaultTokens() {
		dictionary[token] = struct{}{}
	}

	for token := range shutdownBenignTokens {
		assert.Contains(t, dictionary, token, "token is not in the measured dictionary")
		_, isFault := shutdownFaultFamilies[tokenFamily(token)]
		assert.True(t, isFault, "%s would be benign without the exclusion", token)
	}
}

// TestClassifyShutdownTokensExcludesBenignTokens pins both halves of the
// exclusion: a benign token cannot elect a class on its own, and it cannot
// suppress a genuine fault it happens to accompany.
func TestClassifyShutdownTokensExcludesBenignTokens(t *testing.T) {
	for token := range shutdownBenignTokens {
		t.Run(token+" alone stays silent", func(t *testing.T) {
			result, emit := classifyShutdownTokens(pmuBootFaultInfo{Tokens: []string{token}})
			assert.False(t, emit)
			assert.Empty(t, result.Class)
		})

		t.Run(token+" retained beside a real fault", func(t *testing.T) {
			result, emit := classifyShutdownTokens(pmuBootFaultInfo{
				Tokens: []string{token, "uv,vddmain_uvlo"},
			})
			require.True(t, emit)
			assert.Equal(t, shutdownClassPower, result.Class)
			// Excluded from the fault set, still present in the payload.
			assert.Equal(t, []string{"uv,vddmain_uvlo"}, result.FaultTokens)
			assert.Contains(t, result.Tokens, token)
			assert.Contains(t, result.Families, tokenFamily(token))
		})
	}
}

// TestThermalTokenSet pins the 14 thermal tokens the classifier must match.
func TestThermalTokenSet(t *testing.T) {
	var thermal []string
	for _, token := range allPMUFaultTokens() {
		if shutdownFaultFamilies[tokenFamily(token)] == shutdownClassThermal {
			thermal = append(thermal, token)
		}
	}

	assert.ElementsMatch(t, []string{
		"ntc_shdn",
		"ot,overtemp",
		"ot,tdie0_overtemp",
		"ot,tdie1_overtemp",
		"ot,tdie_overtemp",
		"ot,tdie_overtemp_idx0",
		"ot,tdie_overtemp_idx1",
		"ot,tdie_overtemp_idx2",
		"ot,tdie_overtemp_idx3",
		"ot,temp_abs_buck0",
		"ot,temp_abs_buck1",
		"ot,temp_abs_buck2",
		"ot,tsns_overtemp",
		"sochot,reset_in_3",
	}, thermal)
}

func TestTokenFamily(t *testing.T) {
	tests := map[string]string{
		"ot,tdie_overtemp":      "ot",
		"crash,crash0_in":       "crash",
		"ntc_shdn":              "ntc_shdn",
		"otp_crc":               "otp_crc",
		"por":                   "por",
		"rst":                   "rst",
		"sgpio":                 "sgpio",
		"buck_startup_timeout":  "buck",
		"pgood_error_idx1":      "pgood",
		"target_off_shutdown":   "target_off",
		"cp_wdog_expiry":        "cp_wdog_expiry",
		"ldo_dig_ovs":           "ldo_dig_ovs",
		"emerg_shdn":            "emerg_shdn",
		"btn_shdn":              "btn_shdn",
		"unprefixed_and_lonely": "unprefixed_and_lonely",
	}

	for token, family := range tests {
		assert.Equal(t, family, tokenFamily(token), token)
	}
}

// TestShutdownClassPrecedenceIsComplete keeps a newly added classification from
// being unreachable or untitled.
func TestShutdownClassPrecedenceIsComplete(t *testing.T) {
	ranked := make(map[shutdownClass]struct{}, len(shutdownClassPrecedence))
	for _, class := range shutdownClassPrecedence {
		require.NotEqual(t, shutdownClassNone, class)
		_, duplicate := ranked[class]
		require.False(t, duplicate, "%s ranked twice", class)
		ranked[class] = struct{}{}

		assert.NotEmpty(t, shutdownTitles[class], class)
		assert.NotEmpty(t, shutdownMessages[class], class)
	}

	for _, class := range shutdownFaultFamilies {
		assert.Contains(t, ranked, class)
	}
	assert.Len(t, shutdownTitles, len(shutdownClassPrecedence))
	assert.Len(t, shutdownMessages, len(shutdownClassPrecedence))
}

func sortedAscending(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] >= values[index] {
			return false
		}
	}
	return true
}

// allPMUFaultTokens is the complete info-fault_name dictionary published by the
// pmu,abbey and pmu,mosquito services on the measured hardware.
func allPMUFaultTokens() []string {
	return []string{
		"btn_rst,btn_seq_reset",
		"btn_rst,two_finger_rst",
		"btn_shdn",
		"buck_boot_charge",
		"buck_en_err",
		"buck_idemov",
		"buck_low_pwr_err",
		"buck_startup_timeout",
		"cp_wdog_expiry",
		"crash,crash0_in",
		"crash,crash1_in",
		"crash,crash2_in",
		"crash,crash_in",
		"crash,hyp_fw_crash",
		"crash,hyp_hw_crash",
		"dbg_rst,reset_in_2",
		"emerg_shdn",
		"fault,fault_in",
		"ldo_dig_ovs",
		"ntc_shdn",
		"oc,buck_tocp",
		"ot,overtemp",
		"ot,tdie0_overtemp",
		"ot,tdie1_overtemp",
		"ot,tdie_overtemp",
		"ot,tdie_overtemp_idx0",
		"ot,tdie_overtemp_idx1",
		"ot,tdie_overtemp_idx2",
		"ot,tdie_overtemp_idx3",
		"ot,temp_abs_buck0",
		"ot,temp_abs_buck1",
		"ot,temp_abs_buck2",
		"ot,tsns_overtemp",
		"otp_crc",
		"ov,buck_ovp",
		"ov,cp_ovlo",
		"ov,ldo9_ov",
		"ov,ldo9b_ov",
		"ov,vddmain_ovlo",
		"pgood_error_idx0",
		"pgood_error_idx1",
		"por",
		"por,por_vddrtc",
		"por,sw_por",
		"por,vdddig_por",
		"por,vddmac_por",
		"rst",
		"rst,rst_vddrtc",
		"rst_in,reset_in_0",
		"rst_in,reset_in_1_deassert",
		"sgpio",
		"sgpio,sgpio_error",
		"sochot,reset_in_3",
		"spmi,spmi_fault",
		"sstate,button_dfu_recover",
		"sstate,wallet_crash_seq",
		"target_off_conflict",
		"target_off_restart",
		"target_off_shutdown",
		"timeout,crash_timeout",
		"timeout,dblclick_timeout",
		"timeout,power_down_watchdog_timeout",
		"timeout,target_st_wdog_timeout",
		"timeout,watchdog_timeout",
		"timeout,wdog_fw_timeout",
		"uv,cp_uvlo",
		"uv,ext_vddmain_uvlo",
		"uv,ext_vddmain_uvlo_hold",
		"uv,ldo9_uv",
		"uv,ldo9b_uv",
		"uv,pbus_uvfault",
		"uv,pbus_uvlo",
		"uv,por_warn",
		"uv,vdd_boost_uvlo",
		"uv,vddmac_uvlo",
		"uv,vddmain_uvlo",
		"uv,vddmain_uvlo_hold",
		"vddio,vddio_1v2_sgpio0_ok",
		"vddio,vddio_1v2_sgpio1_ok",
		"wdog,reset_in_1",
	}
}
