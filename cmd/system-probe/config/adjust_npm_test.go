// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestAdjustConnectionRollup(t *testing.T) {
	tests := []struct {
		npmEnabled, usmEnabled   bool
		npmAdjusted, usmAdjusted bool
	}{
		{false, false, false, false},
		{true, false, false, false}, // this is the only case where the configs are "adjusted"
		{false, true, false, true},
		{true, true, true, true},
	}

	for _, te := range tests {
		t.Run(fmt.Sprintf("npm_enabled_%t_usm_enabled_%t", te.npmEnabled, te.usmEnabled), func(t *testing.T) {
			config.ResetSystemProbeConfig(t)
			cfg := config.SystemProbe
			cfg.Set(netNS("enable_connection_rollup"), te.npmEnabled, model.SourceUnknown)
			cfg.Set(smNS("enable_connection_rollup"), te.usmEnabled, model.SourceUnknown)
			Adjust(cfg)

			assert.Equal(t, te.npmAdjusted, cfg.GetBool(netNS("enable_connection_rollup")), "adjusted network_config.enable_connection_rollup does not match expected value")
			assert.Equal(t, te.usmAdjusted, cfg.GetBool(smNS("enable_connection_rollup")), "adjusted service_monitoring_config.enable_connection_rollup does not match expected value")
		})
	}
}
