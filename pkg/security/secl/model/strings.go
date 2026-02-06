// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"unicode"

	"golang.org/x/net/publicsuffix"
)

var (
	alphaNumericRange = []*unicode.RangeTable{unicode.L, unicode.Digit}
)

// IsAlphaNumeric returns whether a character is either a digit or a letter
func IsAlphaNumeric(r rune) bool {
	return unicode.IsOneOf(alphaNumericRange, r)
}

// IsPrintable returns whether the string does contain only unicode printable
func IsPrintable(s string) bool {
	for _, c := range s {
		if !unicode.IsOneOf(unicode.PrintRanges, c) {
			return false
		}
	}
	return true
}

// IsPrintableASCII returns whether the string does contain only ASCII char
func IsPrintableASCII(s string) bool {
	for _, c := range s {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && c != '/' && c != ':' && c != '-' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

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
