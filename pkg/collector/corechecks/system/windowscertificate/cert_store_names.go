// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package windowscertificate

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const systemCertificatesRegistryPath = `SOFTWARE\Microsoft\SystemCertificates`

func systemCertificateStoreNames(root registry.Key) ([]string, error) {
	k, err := registry.OpenKey(root, systemCertificatesRegistryPath, registry.READ|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", systemCertificatesRegistryPath, err)
	}
	defer k.Close()
	return k.ReadSubKeyNames(-1)
}

// resolveStoreNames returns the sorted, case-insensitively deduplicated union of
// the explicit store name (if non-empty) and any names from available that match
// at least one of the regexes. Deduplication is case-insensitive because Windows
// certificate store names are case-insensitive: "my" and "MY" refer to the same store.
func resolveStoreNames(explicit string, available []string, regexes []*regexp.Regexp) []string {
	explicit = strings.TrimSpace(explicit)
	seen := make(map[string]struct{}) // keyed by strings.ToLower(name)
	var out []string

	add := func(name string) {
		lower := strings.ToLower(name)
		if _, ok := seen[lower]; !ok {
			seen[lower] = struct{}{}
			out = append(out, name)
		}
	}

	if explicit != "" {
		add(explicit)
	}
	for _, name := range available {
		for _, re := range regexes {
			if re.MatchString(name) {
				add(name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func validateCertificateStoreSelection(c *Config) error {
	store := strings.TrimSpace(c.CertificateStore)
	hasStore := store != ""
	hasRegex := len(c.CertificateStoreRegex) > 0
	if !hasStore && !hasRegex {
		return errors.New("either certificate_store or certificate_store_regex (with at least one pattern) must be set")
	}
	return nil
}

func compileCertificateStoreRegexes(patterns []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for i, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			return nil, fmt.Errorf("certificate_store_regex[%d] must not be empty", i)
		}
		compiled := pat
		// Windows store names are typically upper-case (MY, ROOT). Match patterns case-insensitively
		// unless the user already enabled (?i) at the start of the pattern.
		if !strings.HasPrefix(pat, "(?i)") {
			compiled = "(?i)" + pat
		}
		re, err := regexp.Compile(compiled)
		if err != nil {
			return nil, fmt.Errorf("certificate_store_regex[%d]: invalid regular expression: %w", i, err)
		}
		out = append(out, re)
	}
	return out, nil
}
