// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPublicTLD(t *testing.T) {
	t.Run("valid-fqdn", func(t *testing.T) {
		etldPlusOne := GetPublicTLD("www.yahoo.com")
		assert.Equal(t, "yahoo.com", etldPlusOne)
	})
}

func TestGetPublicTLDs(t *testing.T) {
	t.Run("valid-fqdns", func(t *testing.T) {
		etldPlusOnes := GetPublicTLDs([]string{"www.yahoo.com", "www.google.com", "ftp.yahoo.com"})
		assert.Equal(t, []string{"yahoo.com", "google.com"}, etldPlusOnes)
	})
}
