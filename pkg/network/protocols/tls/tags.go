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

// Constants for tag keys
const (
	TagTLSVersion       = "tls.version:"
	TagTLSCipherSuiteID = "tls.cipher_suite_id:"
	TagTLSClientVersion = "tls.client_version:"
	version10           = "tls_1.0"
	version11           = "tls_1.1"
	version12           = "tls_1.2"
	version13           = "tls_1.3"
)

// Bitmask constants for Offered_versions matching kernelspace definitions
const (
	OfferedTLSVersion10 uint8 = 0x01
	OfferedTLSVersion11 uint8 = 0x02
	OfferedTLSVersion12 uint8 = 0x04
	OfferedTLSVersion13 uint8 = 0x08
)

// VersionTags maps TLS versions to tag names for server chosen version (exported for testing)
var VersionTags = map[uint16]string{
	tls.VersionTLS10: TagTLSVersion + version10,
	tls.VersionTLS11: TagTLSVersion + version11,
	tls.VersionTLS12: TagTLSVersion + version12,
	tls.VersionTLS13: TagTLSVersion + version13,
}

// ClientVersionTags maps TLS versions to tag names for client offered versions (exported for testing)
var ClientVersionTags = map[uint16]string{
	tls.VersionTLS10: TagTLSClientVersion + version10,
	tls.VersionTLS11: TagTLSClientVersion + version11,
	tls.VersionTLS12: TagTLSClientVersion + version12,
	tls.VersionTLS13: TagTLSClientVersion + version13,
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
	if t == nil {
		return true
	}
	return t.ChosenVersion == 0 && t.CipherSuite == 0 && t.OfferedVersions == 0
}

// String returns a string representation of the Tags struct
func (t *Tags) String() string {
	return fmt.Sprintf("ChosenVersion: %d, CipherSuite: %d, OfferedVersions: %d", t.ChosenVersion, t.CipherSuite, t.OfferedVersions)
}

// parseOfferedVersions parses the Offered_versions bitmask into a slice of version strings
func parseOfferedVersions(offeredVersions uint8) []string {
	versions := make([]string, 0, len(offeredVersionBitmask))
	for _, ov := range offeredVersionBitmask {
		if (offeredVersions & ov.bitMask) != 0 {
			if name := ClientVersionTags[ov.version]; name != "" {
				versions = append(versions, name)
			}
		}
	}
	return versions
}

func hexCipherSuiteTag(cipherSuite uint16) string {
	return fmt.Sprintf("%s0x%04X", TagTLSCipherSuiteID, cipherSuite)
}

// GetDynamicTags generates dynamic tags based on TLS information
func (t *Tags) GetDynamicTags() map[string]struct{} {
	if t.IsEmpty() {
		return nil
	}
	tags := make(map[string]struct{})

	// Server chosen version
	if tag, ok := VersionTags[t.ChosenVersion]; ok {
		tags[tag] = struct{}{}
	}

	// Client offered versions
	for _, versionName := range parseOfferedVersions(t.OfferedVersions) {
		tags[versionName] = struct{}{}
	}

	// Cipher suite ID as hex string
	if t.CipherSuite != 0 {
		tags[hexCipherSuiteTag(t.CipherSuite)] = struct{}{}
	}

	return tags
}
