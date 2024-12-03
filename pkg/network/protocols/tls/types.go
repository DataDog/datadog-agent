// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package tls contains definitions and methods related to tags parsed from the TLS handshake
package tls

import (
	"crypto/tls"
	"fmt"
)

// Bitmask constants for Offered_versions matching kernelspace definitions
const (
	OfferedTLSVersion10 uint8 = 0x01
	OfferedTLSVersion11 uint8 = 0x02
	OfferedTLSVersion12 uint8 = 0x04
	OfferedTLSVersion13 uint8 = 0x08
)

// mapping of version constants to their string representations
var tlsVersionNames = map[uint16]string{
	tls.VersionTLS10: "TLS 1.0",
	tls.VersionTLS11: "TLS 1.1",
	tls.VersionTLS12: "TLS 1.2",
	tls.VersionTLS13: "TLS 1.3",
}

// Mapping of offered version bitmasks to version constants
var offeredVersionBitmask = []struct {
	bitMask uint8
	version uint16
}{
	{OfferedTLSVersion10, tls.VersionTLS10},
	{OfferedTLSVersion11, tls.VersionTLS11},
	{OfferedTLSVersion12, tls.VersionTLS12},
	{OfferedTLSVersion13, tls.VersionTLS13},
}

// Constants for tag keys
const (
	TagTLSVersion       = "tls.version:"
	TagTLSCipherSuiteID = "tls.cipher_suite_id:"
	TagTLSClientVersion = "tls.client_version:"
)

// Tags holds the TLS tags. It is used to store the TLS version, cipher suite and offered versions.
// We can't use the struct from eBPF as the definition is shared with windows.
type Tags struct {
	ChosenVersion   uint16
	CipherSuite     uint16
	OfferedVersions uint8
}

// MergeWith merges the tags from another Tags struct into this one
func (t *Tags) MergeWith(that Tags) {
	if t.ChosenVersion == 0 {
		t.ChosenVersion = that.ChosenVersion
	}
	if t.CipherSuite == 0 {
		t.CipherSuite = that.CipherSuite
	}
	if t.OfferedVersions == 0 {
		t.OfferedVersions = that.OfferedVersions
	}

}

// IsEmpty returns true if all fields are zero
func (t *Tags) IsEmpty() bool {
	return t.ChosenVersion == 0 && t.CipherSuite == 0 && t.OfferedVersions == 0
}

// String returns a string representation of the Tags struct
func (t *Tags) String() string {
	return fmt.Sprintf("ChosenVersion: %d, CipherSuite: %d, OfferedVersions: %d", t.ChosenVersion, t.CipherSuite, t.OfferedVersions)
}

// FormatTLSVersion converts a version uint16 to its string representation
func FormatTLSVersion(version uint16) string {
	if name, ok := tlsVersionNames[version]; ok {
		return name
	}
	return ""
}

// parseOfferedVersions parses the Offered_versions bitmask into a slice of version strings
func parseOfferedVersions(offeredVersions uint8) []string {
	versions := []string{}
	for _, ov := range offeredVersionBitmask {
		if (offeredVersions & ov.bitMask) != 0 {
			if name := tlsVersionNames[ov.version]; name != "" {
				versions = append(versions, name)
			}
		}
	}
	return versions
}

// GetTLSDynamicTags generates dynamic tags based on TLS information
func GetTLSDynamicTags(tls *Tags) map[string]struct{} {
	tags := make(map[string]struct{})
	if tls == nil {
		return tags
	}

	// Server chosen version
	if versionName := FormatTLSVersion(tls.ChosenVersion); versionName != "" {
		tags[TagTLSVersion+versionName] = struct{}{}
	}

	// Cipher suite ID as hex string
	if tls.CipherSuite != 0 {
		tags[TagTLSCipherSuiteID+fmt.Sprintf("0x%04X", tls.CipherSuite)] = struct{}{}
	}

	// Client offered versions
	for _, versionName := range parseOfferedVersions(tls.OfferedVersions) {
		tags[TagTLSClientVersion+versionName] = struct{}{}
	}

	return tags
}
