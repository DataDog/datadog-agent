// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package net

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFqdn_InvalidHostnameReturnsInput(t *testing.T) {
	// When hostname cannot be resolved, it should return the original hostname
	// Using .invalid TLD per RFC 2606 - guaranteed to never resolve
	invalidHostname := "nonexistent-host.example.invalid"
	result := Fqdn(invalidHostname)
	assert.Equal(t, invalidHostname, result)
}

func TestFqdn_EmptyHostnameReturnsEmpty(t *testing.T) {
	// Empty hostname should return empty (DNS lookup will fail)
	result := Fqdn("")
	assert.Equal(t, "", result)
}
