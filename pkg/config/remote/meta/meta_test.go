// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package meta

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseProdDirectorVersion(t *testing.T) {
	v, err := parseRootVersion(prodRootDirector)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseProdTUFVersion(t *testing.T) {
	v, err := parseRootVersion(prodRootConfig)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseStagingDirectorVersion(t *testing.T) {
	v, err := parseRootVersion(stagingRootDirector)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseStagingTUFVersion(t *testing.T) {
	v, err := parseRootVersion(stagingRootConfig)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseGovDirectorVersion(t *testing.T) {
	v, err := parseRootVersion(govRootDirector)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseGovTUFVersion(t *testing.T) {
	v, err := parseRootVersion(govRootConfig)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

// The director root metadata must always be version 1. As part of the
// RC protocol, the RC core services sends the Director TUF root to agent
// and tracer clients. It will try to send to those clients all root files
// starting from the version the client reports it has, through the latest
// the core service has gotten from the RC backend.
//
// If we embed a director root greater than 1, the RC backend does not go back
// and fill in versions. Some clients, when they start up, always unconditionally
// report they have version 1. In those circumstances, we'll be unable to find the roots
// earlier than the version we embed, and the request will error out.
func TestDirectorRootMetadataIsVersion1(t *testing.T) {
	for _, tc := range []struct {
		name string
		root []byte
	}{
		{"prod", prodRootDirector},
		{"staging", stagingRootDirector},
		{"gov", govRootDirector},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := parseRootVersion(tc.root)
			require.NoError(t, err)
			require.Equal(t, uint64(1), v, "director root metadata for %q must be version 1", tc.name)
		})
	}
}
