// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packages

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

// TestDDOTProcmgrConfigVariants verifies the generated DDOT process definition leaves the config
// paths as the ${DD_CONF_DIR} placeholder, which the supervising dd-procmgr substitutes at launch
// with its stable or experiment config directory, while the binary path is baked per install tree.
func TestDDOTProcmgrConfigVariants(t *testing.T) {
	tests := []struct {
		name            string
		unitType        embedded.UnitType
		experiment      bool
		wantCommand     string
		wantInventories bool
	}{
		{
			name:        "oci stable",
			unitType:    embedded.UnitTypeOCI,
			wantCommand: "/opt/datadog-packages/datadog-agent/stable/ext/ddot/embedded/bin/otel-agent",
		},
		{
			name:            "oci experiment",
			unitType:        embedded.UnitTypeOCI,
			experiment:      true,
			wantCommand:     "/opt/datadog-packages/datadog-agent/experiment/ext/ddot/embedded/bin/otel-agent",
			wantInventories: true,
		},
		{
			name:        "debrpm stable",
			unitType:    embedded.UnitTypeDebRpm,
			wantCommand: "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent",
		},
		{
			name:            "debrpm experiment",
			unitType:        embedded.UnitTypeDebRpm,
			experiment:      true,
			wantCommand:     "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent",
			wantInventories: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := embedded.GetProcmgrUnit(ddotProcmgrConfigName, tt.unitType, false)
			if tt.experiment {
				raw, err = embedded.GetProcmgrUnit(ddotProcmgrExpConfigName, tt.unitType, false)
			}
			require.NoError(t, err)
			content := string(raw)

			assert.Contains(t, content, "${DD_CONF_DIR}/otel-config.yaml")
			assert.Contains(t, content, "${DD_CONF_DIR}/datadog.yaml")
			assert.Contains(t, content, "command: "+tt.wantCommand)
			// The daemon skips the payload when the binary is absent, so the definition can be
			// shipped unconditionally.
			assert.Contains(t, content, "condition_path_exists: "+tt.wantCommand)

			if tt.wantInventories {
				assert.Contains(t, content, "DD_INVENTORIES_FIRST_RUN_DELAY")
			} else {
				assert.NotContains(t, content, "DD_INVENTORIES_FIRST_RUN_DELAY")
			}
		})
	}
}
