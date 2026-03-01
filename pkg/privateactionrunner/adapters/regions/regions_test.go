// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package regions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetRegionFromDDSite_EU verifies that the European site returns the fixed "eu1" region
// code, which differs from the region-in-subdomain pattern used by US sites.
func TestGetRegionFromDDSite_EU(t *testing.T) {
	assert.Equal(t, "eu1", GetRegionFromDDSite("datadoghq.eu"))
}

// TestGetRegionFromDDSite_US3 verifies that a site of the form "<region>.datadoghq.com"
// returns just the region prefix (e.g., "us3").
func TestGetRegionFromDDSite_US3(t *testing.T) {
	assert.Equal(t, "us3", GetRegionFromDDSite("us3.datadoghq.com"))
}

// TestGetRegionFromDDSite_AP1 verifies that Asia-Pacific site follows the same
// subdomain-as-region convention.
func TestGetRegionFromDDSite_AP1(t *testing.T) {
	assert.Equal(t, "ap1", GetRegionFromDDSite("ap1.datadoghq.com"))
}

// TestGetRegionFromDDSite_DefaultFallback verifies that any site that is neither
// "datadoghq.eu" nor a "*.datadoghq.com" domain falls back to "us1", which is the
// default Datadog region.
func TestGetRegionFromDDSite_DefaultFallback(t *testing.T) {
	assert.Equal(t, "us1", GetRegionFromDDSite("datadoghq.com"))
	assert.Equal(t, "us1", GetRegionFromDDSite("custom.internal.corp"))
	assert.Equal(t, "us1", GetRegionFromDDSite(""))
}

// TestGetRegionFromDDSite_DoesNotMatchSubdomain verifies that "datadoghq.com" (without a
// region prefix) hits the fallback rather than returning an empty string, since
// HasSuffix(".datadoghq.com") does not match "datadoghq.com" itself.
func TestGetRegionFromDDSite_BareDatadoghqCom(t *testing.T) {
	// "datadoghq.com" does not have the suffix ".datadoghq.com" (note the leading dot),
	// so it falls through to the default "us1".
	assert.Equal(t, "us1", GetRegionFromDDSite("datadoghq.com"))
}
