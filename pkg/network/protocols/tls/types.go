// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package tls

import "fmt"

// TLS and SSL version constants
const (
	SSLVersion20 uint16 = 0x0200
	SSLVersion30 uint16 = 0x0300
	TLSVersion10 uint16 = 0x0301
	TLSVersion11 uint16 = 0x0302
	TLSVersion12 uint16 = 0x0303
	TLSVersion13 uint16 = 0x0304
)

// Bitmask constants for Offered_versions
const (
	OfferedSSLVersion20 uint8 = 0x01 // Bit 0
	OfferedSSLVersion30 uint8 = 0x02 // Bit 1
	OfferedTLSVersion10 uint8 = 0x04 // Bit 2
	OfferedTLSVersion11 uint8 = 0x08 // Bit 3
	OfferedTLSVersion12 uint8 = 0x10 // Bit 4
	OfferedTLSVersion13 uint8 = 0x20 // Bit 5
)

// mapping of version constants to their string representations
var tlsVersionNames = map[uint16]string{
	SSLVersion20: "SSL 2.0",
	SSLVersion30: "SSL 3.0",
	TLSVersion10: "TLS 1.0",
	TLSVersion11: "TLS 1.1",
	TLSVersion12: "TLS 1.2",
	TLSVersion13: "TLS 1.3",
}

// Mapping of offered version bitmasks to version constants
var offeredVersionBitmask = []struct {
	bitMask uint8
	version uint16
}{
	{OfferedSSLVersion20, SSLVersion20},
	{OfferedSSLVersion30, SSLVersion30},
	{OfferedTLSVersion10, TLSVersion10},
	{OfferedTLSVersion11, TLSVersion11},
	{OfferedTLSVersion12, TLSVersion12},
	{OfferedTLSVersion13, TLSVersion13},
}

// Constants for tag keys
const (
	TagTLSVersion       = "tls.version:"
	tagTLSCipherSuiteID = "tls.cipher_suite_id:"
	tagTLSClientVersion = "tls.client_version:"
)

type Tags struct {
	ChosenVersion   uint16
	CipherSuite     uint16
	OfferedVersions uint8
}

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

func (t *Tags) IsEmpty() bool {
	return t.ChosenVersion == 0 && t.CipherSuite == 0 && t.OfferedVersions == 0
}

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
		tags[tagTLSCipherSuiteID+fmt.Sprintf("0x%04X", tls.CipherSuite)] = struct{}{}
	}

	// Client offered versions
	for _, versionName := range parseOfferedVersions(tls.OfferedVersions) {
		tags[tagTLSClientVersion+versionName] = struct{}{}
	}

	return tags
}
