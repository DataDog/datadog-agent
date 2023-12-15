// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDnsHelpers_validateDNSName(t *testing.T) {
	type test struct {
		testName string
		dnsName  string
		isValid  bool
	}

	tests := []test{
		// test valid dns names
		{
			testName: "test_ok_1",
			dnsName:  "foo.bar",
			isValid:  true,
		},
		{
			testName: "test_ok_2",
			dnsName:  "a.b",
			isValid:  true,
		},
		{
			testName: "test_ok_3",
			dnsName:  "a.b.c",
			isValid:  true,
		},
		{
			testName: "test_ok_max_total_length",
			dnsName:  "0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.253",
			isValid:  true,
		},
		{
			testName: "test_ok_max_domain_length",
			dnsName:  "012345678901234567890123456789012345678901234567890123456789012.com",
			isValid:  true,
		},
		{
			testName: "test_ok_single_char",
			dnsName:  "a",
			isValid:  true,
		},
		{
			testName: "test_ok_hostname",
			dnsName:  "localhost",
			isValid:  true,
		},

		// test invalid dns names
		{
			testName: "test_ko_2",
			dnsName:  "a.b.",
			isValid:  false,
		},
		{
			testName: "test_ko_3",
			dnsName:  ".a.b",
			isValid:  false,
		},
		{
			testName: "test_ko_4",
			dnsName:  ".a.b.",
			isValid:  false,
		},
		{
			testName: "test_ko_5",
			dnsName:  "a..b",
			isValid:  false,
		},
		{
			testName: "test_ko_7",
			dnsName:  "...",
			isValid:  false,
		},
		{
			testName: "test_ko_8",
			dnsName:  "a.b.c..d.e.f",
			isValid:  false,
		},
		{
			testName: "test_ko_max_total_length",
			dnsName:  "0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.0.2.4.6.8.2530",
			isValid:  false,
		},
		{
			testName: "test_ko_max_domain_length",
			dnsName:  "0123456789012345678901234567890123456789012345678901234567890123.com",
			isValid:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			err := validateDNSName(test.dnsName)
			assert.Equal(t, test.isValid, err == nil)
		})
	}
}
