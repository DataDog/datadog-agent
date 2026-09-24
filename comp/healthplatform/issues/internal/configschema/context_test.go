// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configschema

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestResolveDefault(t *testing.T) {
	cfg := config.NewMockFromYAML(t, "agent_ipc: invalid")
	for _, tc := range []struct{ path, status string }{
		{"/agent_ipc", "none"},
		{"/unknown_setting", "unknown"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			status, value := resolveDefault(cfg, tc.path)
			assert.Equal(t, tc.status, status)
			assert.Nil(t, value)
		})
	}
}
