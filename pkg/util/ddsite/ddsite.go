// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddsite provides functions for identifying and classifying Datadog
// domains and sites. It centralizes the regex patterns and helpers so that
// callers never need to define their own DD-domain regexes.
package ddsite

import (
	"net/url"
	"regexp"
	"strings"
)

// GovDomainPattern is a regex fragment matching gov-cloud Datadog domains.
const GovDomainPattern = `ddog-gov\.(?:com|mil)`

// DDDomainPattern is a regex fragment matching all known Datadog domains
// (prod, staging, and gov-cloud).
const DDDomainPattern = `datad(?:oghq|0g)\.(?:com|eu)|` + GovDomainPattern

// govSiteRe matches a gov-cloud site value: bare domain or with a datacenter prefix.
var govSiteRe = regexp.MustCompile(`^(.+\.)?` + GovDomainPattern + `$`)

// govURLRe matches a full URL pointing to a gov-cloud domain.
var govURLRe = regexp.MustCompile(`^https?://.+\.` + GovDomainPattern)

// knownSiteRe matches any known DD site value (with optional datacenter prefix).
var knownSiteRe = regexp.MustCompile(`(?:` + DDDomainPattern + `)$`)

// sitePattern matches an optional datacenter subdomain followed by a known DD domain.
const sitePattern = `([a-z]{2,}\d{1,2}\.)?(` + DDDomainPattern + `)`

// siteFromHostnameRe extracts the DD site from a hostname. The (?:^|\.)
// prefix ensures the match starts at a label boundary so that, e.g.,
// "notdatadoghq.com" is not mistaken for "datadoghq.com".
var siteFromHostnameRe = regexp.MustCompile(`(?:^|\.)` + sitePattern + `\.?$`)

// IsGovSite reports whether site is a gov-cloud Datadog domain.
// It accepts both bare domains ("ddog-gov.com") and datacenter-prefixed
// forms ("xxxx99.ddog-gov.mil").
func IsGovSite(site string) bool {
	return govSiteRe.MatchString(site)
}

// IsGovURL reports whether rawURL points to a gov-cloud Datadog domain.
func IsGovURL(rawURL string) bool {
	return govURLRe.MatchString(rawURL)
}

// IsKnownSite reports whether site is a known Datadog domain (prod, staging,
// or gov-cloud), with or without a datacenter prefix.
func IsKnownSite(site string) bool {
	return knownSiteRe.MatchString(site)
}

// IsKnownHost reports whether hostname belongs to a known Datadog domain.
// The hostname should not include a scheme or path.
func IsKnownHost(hostname string) bool {
	return ExtractSiteFromHostname(hostname) != ""
}

// ExtractSiteFromHostname extracts the Datadog site from a hostname.
// For example:
//
//	"app.us3.datadoghq.com"          -> "us3.datadoghq.com"
//	"intake.profile.ddog-gov.com"    -> "ddog-gov.com"
//	"custom.example.com"             -> ""
//
// Trailing dots are stripped before matching.
func ExtractSiteFromHostname(hostname string) string {
	hostname = strings.ToLower(strings.TrimRight(hostname, "."))
	if hostname == "" {
		return ""
	}
	matches := siteFromHostnameRe.FindStringSubmatch(hostname)
	if matches == nil {
		return ""
	}
	// matches[1] = DC label with trailing dot (e.g. "us3.") or ""
	// matches[2] = known domain (e.g. "datadoghq.com")
	return matches[1] + matches[2]
}

// ExtractSiteFromURL extracts the Datadog site from a full URL.
// For example:
//
//	"https://intake.profile.us3.datadoghq.com/v1/input" -> "us3.datadoghq.com"
//	"https://intake.profile.datadoghq.com/v1/input"     -> "datadoghq.com"
//	"https://intake.profile.datadoghq.eu/v1/input"      -> "datadoghq.eu"
//
// Returns an empty string if the URL cannot be parsed or does not contain a
// recognized Datadog domain.
func ExtractSiteFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return ExtractSiteFromHostname(u.Hostname())
}

// GetAPIDomain transforms a DD endpoint URL into its API-domain equivalent by
// replacing the subdomain(s) with "api". Trailing dots in the hostname are
// preserved. Returns the input unchanged if it is not a recognized DD domain.
//
// Examples:
//
//	"https://agent.us3.datadoghq.com"  -> "https://api.us3.datadoghq.com"
//	"https://app.ddog-gov.com."        -> "https://api.ddog-gov.com."
//	"https://custom.example.com"       -> "https://custom.example.com"
func GetAPIDomain(endpoint string) string {
	toParse := endpoint
	if !strings.Contains(toParse, "://") {
		toParse = "https://" + toParse
	}
	u, err := url.Parse(toParse)
	if err != nil {
		return endpoint
	}

	site := ExtractSiteFromHostname(u.Hostname())
	if site == "" {
		return endpoint
	}

	trailingDot := ""
	if strings.HasSuffix(u.Host, ".") {
		trailingDot = "."
	}

	return "https://api." + site + trailingDot
}
