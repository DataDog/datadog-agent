// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winsoftware

import (
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
)

func TestSoftwareInventoryModuleConfig(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		cfg := configmock.NewSystemProbe(t)
		cfg.SetWithoutSource("software_inventory.enabled", true)
		assert.True(t, cfg.GetBool("software_inventory.enabled"))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		t.Setenv("DD_SOFTWARE_INVENTORY_ENABLED", "true")
		cfg := configmock.NewSystemProbe(t)
		assert.True(t, cfg.GetBool("software_inventory.enabled"))
	})

	t.Run("default disabled", func(t *testing.T) {
		cfg := configmock.NewSystemProbe(t)
		assert.False(t, cfg.GetBool("software_inventory.enabled"))
	})

	t.Run("explicitly disabled", func(t *testing.T) {
		t.Setenv("DD_SOFTWARE_INVENTORY_ENABLED", "false")
		cfg := configmock.NewSystemProbe(t)
		assert.False(t, cfg.GetBool("software_inventory.enabled"))
	})
}
