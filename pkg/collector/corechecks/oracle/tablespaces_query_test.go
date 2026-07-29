// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTablespaceQuerySelection pins down which query variant is issued for each
// combination of compatibility mode, multitenancy and database version. The
// non-CDB rows are the regression this guards: a database with CDB='NO' has a
// single container, so the CDB_* variants must never be selected for it.
func TestTablespaceQuerySelection(t *testing.T) {
	tests := []struct {
		name            string
		legacy          bool
		multitenant     bool
		dbVersion       string
		expectedUsage   string
		expectedMaxSize string
	}{
		{
			name:            "cdb on a multitenant-capable version uses the container queries",
			multitenant:     true,
			dbVersion:       "19.0.0.0.0",
			expectedUsage:   tablespaceQuery12,
			expectedMaxSize: maxSizeQuery12,
		},
		{
			name:            "non-cdb on a multitenant-capable version uses the plain queries",
			multitenant:     false,
			dbVersion:       "19.0.0.0.0",
			expectedUsage:   tablespaceQuery11,
			expectedMaxSize: maxSizeQuery11,
		},
		{
			name:            "non-cdb on a pre-multitenant version uses the plain queries",
			multitenant:     false,
			dbVersion:       "11.2.0.4.0",
			expectedUsage:   tablespaceQuery11,
			expectedMaxSize: maxSizeQuery11,
		},
		{
			name:            "pre-multitenant version wins over the multitenant flag",
			multitenant:     true,
			dbVersion:       "11.2.0.4.0",
			expectedUsage:   tablespaceQuery11,
			expectedMaxSize: maxSizeQuery11,
		},
		{
			name:            "legacy compatibility mode always uses the plain queries",
			legacy:          true,
			multitenant:     true,
			dbVersion:       "19.0.0.0.0",
			expectedUsage:   tablespaceQuery11,
			expectedMaxSize: maxSizeQuery11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Check{
				legacyIntegrationCompatibilityMode: tt.legacy,
				multitenant:                        tt.multitenant,
				dbVersion:                          tt.dbVersion,
			}
			usage, maxSize := c.tablespaceQueries()
			assert.Equal(t, tt.expectedUsage, usage)
			assert.Equal(t, tt.expectedMaxSize, maxSize)
		})
	}
}

// TestNonCdbTablespaceQueriesAvoidContainerViews is a class-level guard: whatever the
// non-CDB variants grow into, they must not reach for container-scoped objects.
func TestNonCdbTablespaceQueriesAvoidContainerViews(t *testing.T) {
	for _, q := range []string{tablespaceQuery11, maxSizeQuery11} {
		lowered := strings.ToLower(q)
		assert.NotContains(t, lowered, "cdb_", "non-CDB query must not read CDB_* views:\n%s", q)
		assert.NotContains(t, lowered, "v$containers", "non-CDB query must not read v$containers:\n%s", q)
		assert.NotContains(t, lowered, "containers(", "non-CDB query must not use the CONTAINERS() clause:\n%s", q)
	}
}
