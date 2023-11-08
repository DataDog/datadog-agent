// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestInventoryEnabled(t *testing.T) {
	m := config.Mock(t)

	m.SetWithoutSource("enable_metadata_collection", false)
	m.SetWithoutSource("inventories_enabled", true)
	assert.False(t, InventoryEnabled(m))

	m.SetWithoutSource("enable_metadata_collection", true)
	m.SetWithoutSource("inventories_enabled", false)
	assert.False(t, InventoryEnabled(m))

	m.SetWithoutSource("enable_metadata_collection", true)
	m.SetWithoutSource("inventories_enabled", true)
	assert.True(t, InventoryEnabled(m))
}
