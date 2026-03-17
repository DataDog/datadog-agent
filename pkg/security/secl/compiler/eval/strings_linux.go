// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && seclmax) || (linux && test)

// Package eval holds eval related files
package eval

import (
	"slices"

	"github.com/weppos/publicsuffix-go/publicsuffix"
)

// EffectiveTLDPlusOneWithFallback returns the effective top level domain plus one more
// label. It calls the publicsuffix.DomainFromListWithOptions function.
// Fallback: if the publicsuffix.DomainFromListWithOptions function returns an error or an empty
// string, it will return the full domain.
func EffectiveTLDPlusOneWithFallback(domain string) string {
	rootDomain, err := publicsuffix.DomainFromListWithOptions(publicsuffix.DefaultList, domain, &publicsuffix.FindOptions{IgnorePrivate: true})

	if err != nil || rootDomain == "" {
		// Fallback, can't get the root domain, return full domain
		return domain
	}
	return rootDomain
}

// GetPublicTLD returns the public top-level domain (eTLD+1) from an FQDN.
// For example: "www.google.com" returns "google.com", "www.abc.co.uk" returns "abc.co.uk".
func GetPublicTLD(fqdn string) string {
	return EffectiveTLDPlusOneWithFallback(fqdn)
}

// GetPublicTLDs returns the public top-level domains (eTLD+1) from a slice of FQDNs.
// For example: ["www.google.com", "www.abc.co.uk"] returns ["google.com", "abc.co.uk"].
// Removes duplicates.
func GetPublicTLDs(fqdns []string) []string {
	var etldPlusOnes []string
	for _, fqdn := range fqdns {
		etldPlusOne := EffectiveTLDPlusOneWithFallback(fqdn)
		if !slices.Contains(etldPlusOnes, etldPlusOne) {
			etldPlusOnes = append(etldPlusOnes, etldPlusOne)
		}
	}
	return etldPlusOnes
}
