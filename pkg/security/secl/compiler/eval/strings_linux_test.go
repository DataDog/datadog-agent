// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weppos/publicsuffix-go/publicsuffix"
)

func TestGetPublicTLD(t *testing.T) {
	t.Run("valid-fqdn", func(t *testing.T) {
		domainTested := "www.yahoo.com"
		etldPlusOne := GetPublicTLD(domainTested)
		assert.Equal(t, "yahoo.com", etldPlusOne)
		// Check we did not hit the fallback //
		rootDomain, err := publicsuffix.DomainFromListWithOptions(publicsuffix.DefaultList, domainTested, &publicsuffix.FindOptions{IgnorePrivate: true})
		assert.True(t, err == nil, "Should not have failed to get root domain, got %q err=%v", rootDomain, err)
	})
	t.Run("dont-return-private", func(t *testing.T) {
		domainTested := "elb.us-east-1.amazonaws.com"
		etldPlusOne := GetPublicTLD(domainTested)
		fmt.Printf("etldPlusOne: %s\n", etldPlusOne)
		assert.Equal(t, "amazonaws.com", etldPlusOne)
		// Check we did not hit the fallback
		rootDomain, err := publicsuffix.DomainFromListWithOptions(publicsuffix.DefaultList, domainTested, &publicsuffix.FindOptions{IgnorePrivate: true})
		assert.True(t, err == nil, "Should not have failed to get root domain, got %q err=%v", rootDomain, err)
	})
	t.Run("longer-fqdn", func(t *testing.T) {
		domainTested := "www.abc.gov.uk"
		etldPlusOne := GetPublicTLD(domainTested)
		assert.Equal(t, "abc.gov.uk", etldPlusOne)
		// Check we did not hit the fallback
		rootDomain, err := publicsuffix.DomainFromListWithOptions(publicsuffix.DefaultList, domainTested, &publicsuffix.FindOptions{IgnorePrivate: true})
		assert.True(t, err == nil, "Should not have failed to get root domain, got %q err=%v", rootDomain, err)

	})
	t.Run("domain-is-root-domain", func(t *testing.T) {
		domainTested := "abc.gov.uk"
		etldPlusOne := GetPublicTLD(domainTested)
		assert.Equal(t, "abc.gov.uk", etldPlusOne)
		// Check we did not hit the fallback
		rootDomain, err := publicsuffix.DomainFromListWithOptions(publicsuffix.DefaultList, domainTested, &publicsuffix.FindOptions{IgnorePrivate: true})
		assert.True(t, err == nil, "Should not have failed to get root domain, got %q err=%v", rootDomain, err)
	})
	t.Run("invalid-fqdn", func(t *testing.T) {
		domainTested := "no-dot"
		etldPlusOne := GetPublicTLD(domainTested)
		assert.Equal(t, "no-dot", etldPlusOne)
		// Check we hit the fallback
		rootDomain, err := publicsuffix.DomainFromListWithOptions(publicsuffix.DefaultList, domainTested, &publicsuffix.FindOptions{IgnorePrivate: true})
		assert.True(t, err != nil || rootDomain == "", "Should have failed to get root domain, got %q err=%v", rootDomain, err)
	})
}

func TestGetPublicTLDs(t *testing.T) {
	t.Run("valid-fqdns", func(t *testing.T) {
		etldPlusOnes := GetPublicTLDs([]string{"www.yahoo.com", "elb.us-east-1.amazonaws.com", "ftp.yahoo.com", "s3.us-east-1.amazonaws.com"})
		assert.Equal(t, []string{"yahoo.com", "amazonaws.com"}, etldPlusOnes)
	})
	t.Run("invalid-fqdn", func(t *testing.T) {
		etldPlusOnes := GetPublicTLDs([]string{"no-dot"})
		assert.Equal(t, []string{"no-dot"}, etldPlusOnes)
	})
	t.Run("invalid-multiple-fqdns", func(t *testing.T) {
		etldPlusOnes := GetPublicTLDs([]string{"no-dot", "one-dot.com"})
		assert.Equal(t, []string{"no-dot", "one-dot.com"}, etldPlusOnes)
	})
}
