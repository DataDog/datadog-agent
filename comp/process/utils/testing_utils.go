// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"go.uber.org/fx"

	configComp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// DisableContainerFeatures initializes the config with an empty feature map. This prevents the container check from running
// in tests.
var DisableContainerFeatures = fx.Invoke(func(t testing.TB, _ configComp.Component) {
	config.SetDetectedFeatures(config.FeatureMap{})
	t.Cleanup(func() { config.SetDetectedFeatures(nil) })
})
