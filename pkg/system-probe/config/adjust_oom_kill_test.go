// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestAdjustOOMKill(t *testing.T) {
	tests := []struct {
		name     string
		newValue *bool
		oldValue *bool
		expected bool
	}{
		{name: "neither set", expected: false},
		{name: "only new key", newValue: new(true), expected: true},
		{name: "only old key", oldValue: new(true), expected: true},
		{name: "old key wins over new key", newValue: new(false), oldValue: new(true), expected: true},
		{name: "old key wins when it is false", newValue: new(true), oldValue: new(false), expected: false},
	}

	for _, te := range tests {
		t.Run(te.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			if te.newValue != nil {
				cfg.Set(oomKillNS("enabled"), *te.newValue, model.SourceFile)
			}
			if te.oldValue != nil {
				cfg.Set(spNS("enable_oom_kill"), *te.oldValue, model.SourceFile)
			}
			Adjust(cfg)

			assert.Equal(t, te.expected, cfg.GetBool(oomKillNS("enabled")))
			assert.Equal(t, te.expected, cfg.GetBool(spNS("enable_oom_kill")))
		})
	}
}
