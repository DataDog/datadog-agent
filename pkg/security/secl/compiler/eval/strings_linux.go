// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && seclmax) || (linux && test)

// Package eval holds eval related files
package eval

import (
	"slices"

	"golang.org/x/net/publicsuffix"
)

// GetPublicTLD returns the public top-level domain (eTLD+1) from an FQDN.
// For example: "www.google.com" returns "google.com", "www.abc.co.uk" returns "abc.co.uk".
// If the input is invalid or cannot be parsed, it returns the input unchanged.
func GetPublicTLD(fqdn string) string {
	etldPlusOne, err := publicsuffix.EffectiveTLDPlusOne(fqdn)
	if err != nil {
		return fqdn
	}
	return etldPlusOne
}

// GetPublicTLDs returns the public top-level domains (eTLD+1) from a slice of FQDNs.
func GetPublicTLDs(fqdns []string) []string {
	var etldPlusOnes []string
	for _, fqdn := range fqdns {
		etldPlusOne, err := publicsuffix.EffectiveTLDPlusOne(fqdn)
		if err == nil && !slices.Contains(etldPlusOnes, etldPlusOne) {
			etldPlusOnes = append(etldPlusOnes, etldPlusOne)
		}
	}
	return etldPlusOnes
}
