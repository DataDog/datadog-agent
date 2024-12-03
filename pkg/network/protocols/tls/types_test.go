// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package tls

import (
	"crypto/tls"
	"fmt"
	"reflect"
	"testing"
)

func TestFormatTLSVersion(t *testing.T) {
	tests := []struct {
		version  uint16
		expected string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0xFFFF, ""}, // Unknown version
		{0x0000, ""}, // Zero value
		{0x0305, ""}, // Version just above known versions
		{0x01FF, ""}, // Random unknown version
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Version_0x%04X", test.version), func(t *testing.T) {
			result := FormatTLSVersion(test.version)
			if result != test.expected {
				t.Errorf("FormatTLSVersion(0x%04X) = %q; want %q", test.version, result, test.expected)
			}
		})
	}
}

func TestParseOfferedVersions(t *testing.T) {
	tests := []struct {
		offeredVersions uint8
		expected        []string
	}{
		{0x00, []string{}}, // No versions offered
		{OfferedTLSVersion10, []string{"TLS 1.0"}},
		{OfferedTLSVersion11, []string{"TLS 1.1"}},
		{OfferedTLSVersion12, []string{"TLS 1.2"}},
		{OfferedTLSVersion13, []string{"TLS 1.3"}},
		{OfferedTLSVersion10 | OfferedTLSVersion12, []string{"TLS 1.0", "TLS 1.2"}},
		{OfferedTLSVersion11 | OfferedTLSVersion13, []string{"TLS 1.1", "TLS 1.3"}},
		{0xFF, []string{"TLS 1.0", "TLS 1.1", "TLS 1.2", "TLS 1.3"}}, // All bits set
		{0x40, []string{}}, // Undefined bit set
		{0x80, []string{}}, // Undefined bit set
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("OfferedVersions_0x%02X", test.offeredVersions), func(t *testing.T) {
			result := parseOfferedVersions(test.offeredVersions)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("parseOfferedVersions(0x%02X) = %v; want %v", test.offeredVersions, result, test.expected)
			}
		})
	}
}

func TestGetTLSDynamicTags(t *testing.T) {
	tests := []struct {
		name     string
		tlsTags  *Tags
		expected map[string]struct{}
	}{
		{
			name:     "Nil_TLSTags",
			tlsTags:  nil,
			expected: map[string]struct{}{},
		},
		{
			name: "All_Fields_Populated",
			tlsTags: &Tags{
				ChosenVersion:   tls.VersionTLS12,
				CipherSuite:     0x009C,
				OfferedVersions: OfferedTLSVersion11 | OfferedTLSVersion12,
			},
			expected: map[string]struct{}{
				"tls.version:TLS 1.2":        {},
				"tls.cipher_suite_id:0x009C": {},
				"tls.client_version:TLS 1.1": {},
				"tls.client_version:TLS 1.2": {},
			},
		},
		{
			name: "Unknown_Chosen_Version",
			tlsTags: &Tags{
				ChosenVersion:   0xFFFF, // Unknown version
				CipherSuite:     0x00FF,
				OfferedVersions: OfferedTLSVersion13,
			},
			expected: map[string]struct{}{
				"tls.cipher_suite_id:0x00FF": {},
				"tls.client_version:TLS 1.3": {},
			},
		},
		{
			name: "No_Offered_Versions",
			tlsTags: &Tags{
				ChosenVersion:   tls.VersionTLS13,
				CipherSuite:     0x1301,
				OfferedVersions: 0x00,
			},
			expected: map[string]struct{}{
				"tls.version:TLS 1.3":        {},
				"tls.cipher_suite_id:0x1301": {},
			},
		},
		{
			name: "Zero_Cipher_Suite",
			tlsTags: &Tags{
				ChosenVersion:   tls.VersionTLS10,
				OfferedVersions: OfferedTLSVersion10,
			},
			expected: map[string]struct{}{
				"tls.version:TLS 1.0":        {},
				"tls.client_version:TLS 1.0": {},
			},
		},
		{
			name: "All_Bits_Set_In_Offered_Versions",
			tlsTags: &Tags{
				ChosenVersion:   tls.VersionTLS12,
				CipherSuite:     0xC02F,
				OfferedVersions: 0xFF, // All bits set
			},
			expected: map[string]struct{}{
				"tls.version:TLS 1.2":        {},
				"tls.cipher_suite_id:0xC02F": {},
				"tls.client_version:TLS 1.0": {},
				"tls.client_version:TLS 1.1": {},
				"tls.client_version:TLS 1.2": {},
				"tls.client_version:TLS 1.3": {},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := GetTLSDynamicTags(test.tlsTags)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("GetTLSDynamicTags(%v) = %v; want %v", test.tlsTags, result, test.expected)
			}
		})
	}
}
