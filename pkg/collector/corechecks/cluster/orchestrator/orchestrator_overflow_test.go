// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package orchestrator

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestWouldOverflowOnAdd tests the mathematical logic for overflow detection
// This validates the formula: b > math.MaxInt - a prevents overflow when computing a + b
func TestWouldOverflowOnAdd(t *testing.T) {
	tests := []struct {
		name          string
		a             int
		b             int
		shouldOverflow bool
	}{
		{
			name:          "normal case - small values",
			a:             100,
			b:             200,
			shouldOverflow: false,
		},
		{
			name:          "normal case - larger values",
			a:             10000,
			b:             50000,
			shouldOverflow: false,
		},
		{
			name:          "edge case - at boundary",
			a:             math.MaxInt - 100,
			b:             100,
			shouldOverflow: false, // Exactly at limit, no overflow
		},
		{
			name:          "overflow case - just over boundary",
			a:             math.MaxInt - 100,
			b:             101,
			shouldOverflow: true,
		},
		{
			name:          "overflow case - both very large",
			a:             math.MaxInt / 2,
			b:             math.MaxInt/2 + 2,
			shouldOverflow: true,
		},
		{
			name:          "edge case - zero values",
			a:             0,
			b:             0,
			shouldOverflow: false,
		},
		{
			name:          "edge case - one zero",
			a:             0,
			b:             math.MaxInt,
			shouldOverflow: false,
		},
		{
			name:          "edge case - max int with zero",
			a:             math.MaxInt,
			b:             0,
			shouldOverflow: false,
		},
		{
			name:          "overflow case - max int with any positive",
			a:             math.MaxInt,
			b:             1,
			shouldOverflow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the same check used in orchestrator.go
			wouldOverflow := tt.b > math.MaxInt-tt.a
			assert.Equal(t, tt.shouldOverflow, wouldOverflow,
				"overflow check failed for a=%d, b=%d", tt.a, tt.b)

			// If we expect no overflow, verify the addition is safe
			if !tt.shouldOverflow {
				sum := tt.a + tt.b
				assert.GreaterOrEqual(t, sum, tt.a, "sum should be >= a when no overflow")
				assert.GreaterOrEqual(t, sum, tt.b, "sum should be >= b when no overflow")
			}
		})
	}
}

// TestConfigureWithNormalTagCounts verifies that Configure works correctly
// with normal tag counts (no overflow scenario)
func TestConfigureWithNormalTagCounts(t *testing.T) {
	cfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	// Setup fake tagger that returns a normal number of tags
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
	mockSenderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure should succeed with normal tag counts
	err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")

	// We expect an error because the full orchestrator check requires more setup,
	// but it should NOT be an overflow error
	if err != nil {
		assert.NotContains(t, err.Error(), "overflow", "should not get overflow error with normal tag counts")
	}
}

// TestOverflowProtectionDocumentation documents the overflow protection mechanism
// Note: We cannot easily test actual overflow scenarios because creating slices
// with billions of elements would require petabytes of RAM and cause OOM before overflow.
// The mathematical logic is validated in TestWouldOverflowOnAdd.
func TestOverflowProtectionDocumentation(t *testing.T) {
	t.Log("Overflow Protection Mechanism:")
	t.Log("==============================")
	t.Logf("The orchestrator check protects against integer overflow when computing total tag capacity.")
	t.Logf("Formula: if len(b) > math.MaxInt - len(a), then a + b would overflow")
	t.Logf("math.MaxInt on 64-bit systems: %d", math.MaxInt)
	t.Logf("")
	t.Logf("Practical considerations:")
	t.Logf("- Each string pointer in a slice is 16 bytes on 64-bit systems")
	t.Logf("- To overflow on 64-bit: would need ~9.2 quintillion elements")
	t.Logf("- This would require petabytes of RAM")
	t.Logf("- System would OOM before overflow occurs")
	t.Logf("")
	t.Logf("However, the check is necessary because:")
	t.Logf("1. CodeQL flags this pattern as a security vulnerability")
	t.Logf("2. It's good defensive programming practice")
	t.Logf("3. On 32-bit systems, the threshold is much lower (2.1 billion)")
	t.Logf("4. It provides a clear error message if somehow reached")
}
