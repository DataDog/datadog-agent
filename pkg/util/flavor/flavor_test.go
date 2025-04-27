// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flavor

import (
	"fmt"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
)

func TestGetHumanReadableFlavor(t *testing.T) {
	// NOTE: This constructor is required to setup the global config as
	// a "mock" config that is using the "dynamic schema". Otherwise the function
	// SetFlavor in flavor.go will fail to modify the config due to its static schema.
	// TODO: Improve this by making flavor into a component that doesn't use
	// global state and doesn't call SetDefault.
	_ = configmock.New(t)
	for k, v := range agentFlavors {
		t.Run(fmt.Sprintf("%s: %s", k, v), func(t *testing.T) {
			SetFlavor(k)

			assert.Equal(t, v, GetHumanReadableFlavor())
		})
	}

	t.Run("Unknown Agent", func(t *testing.T) {
		SetFlavor("foo")

		assert.Equal(t, "Unknown Agent", GetHumanReadableFlavor())
	})
}
