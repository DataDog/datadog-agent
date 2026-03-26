// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver && cel && test

package autodiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func TestProcessNewConfigCELCompileErrorStoredInStats(t *testing.T) {
	_, ac := getResolveTestConfig(t)

	// Reset errorStats to ensure a clean state
	errorStats = newAcErrorStats()

	invalidTpl := integration.Config{
		Name: "bad-cel-check",
		CELSelector: workloadfilter.Rules{
			Containers: []string{`this is not valid CEL !!!`},
		},
	}

	changes := ac.processNewConfig(invalidTpl)

	// processNewConfig should return empty changes when config cannot be initialized
	assert.Empty(t, changes.Schedule)
	assert.Empty(t, changes.Unschedule)

	// The CEL compile error should be stored in errorStats
	errors := GetConfigErrors()
	assert.Contains(t, errors, "bad-cel-check")
}
