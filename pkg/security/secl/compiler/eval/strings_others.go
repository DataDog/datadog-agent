// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux || (linux && !seclmax && !test)

// Package eval holds eval related files
package eval

// GetPublicTLD returns the public top-level domain (eTLD+1) from an FQDN.
// For example: "www.google.com" returns "google.com", "www.abc.co.uk" returns "abc.co.uk".
// If the input is invalid or cannot be parsed, it returns the input unchanged.
func GetPublicTLD(fqdn string) string {
	return fqdn
}

func GetPublicTLDs(fqdns []string) []string {
	return fqdns
}
