// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSystemFQDNFromCNAME(t *testing.T) {
	mockOsHostname(t, "aix-host", nil)
	lookupCNAME := func(_ context.Context, hostname string) (string, error) {
		assert.Equal(t, "aix-host", hostname)
		return "aix-host.example.com.", nil
	}

	fqdn, err := getSystemFQDNFromCNAME(lookupCNAME)
	require.NoError(t, err)
	assert.Equal(t, "aix-host.example.com", fqdn)
}

func TestGetSystemFQDNFromCNAMELookupError(t *testing.T) {
	mockOsHostname(t, "aix-host", nil)
	errExpected := errors.New("hostname lookup unavailable")
	lookupCNAME := func(context.Context, string) (string, error) {
		return "", errExpected
	}

	_, err := getSystemFQDNFromCNAME(lookupCNAME)
	require.ErrorIs(t, err, errExpected)
}

func TestGetSystemFQDNFromCNAMEHostnameError(t *testing.T) {
	errExpected := errors.New("hostname unavailable")
	mockOsHostname(t, "", errExpected)
	lookupCNAME := func(context.Context, string) (string, error) {
		t.Fatal("LookupCNAME must not be called when os.Hostname fails")
		return "", nil
	}

	_, err := getSystemFQDNFromCNAME(lookupCNAME)
	require.ErrorIs(t, err, errExpected)
}

func mockOsHostname(t *testing.T, hostname string, err error) {
	t.Helper()

	origOSHostname := osHostname
	t.Cleanup(func() { osHostname = origOSHostname })

	osHostname = func() (string, error) {
		return hostname, err
	}
}
