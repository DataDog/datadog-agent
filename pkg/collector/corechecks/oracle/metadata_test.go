// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Database instance metadata needs one authoritative CDB to scope a selected
// raw activity PDB to the cdb.pdb dimension used by query/procedure metrics.
func TestDatabaseInstanceDiscoveredCDB(t *testing.T) {
	c, _, sender := newSamplingCheck(t)
	c.cdbName = "discovered"
	c.config.Tags = []string{"env:test", "cdb:wrong", "CDB:also-wrong", "service:database"}
	require.NoError(t, sendDbInstanceMetadata(c))
	var payload dbInstanceEvent
	for _, call := range sender.Calls {
		if call.Method == "EventPlatformEvent" && call.Arguments[1] == "dbm-metadata" {
			require.NoError(t, json.Unmarshal(call.Arguments[0].([]byte), &payload))
		}
	}
	assert.ElementsMatch(t, []string{"env:test", "service:database", "cdb:discovered"}, payload.Tags)
	assert.Equal(t, "database_instance", payload.Kind)
	assert.Equal(t, []string{"env:test", "cdb:wrong", "CDB:also-wrong", "service:database"}, c.config.Tags)
}
