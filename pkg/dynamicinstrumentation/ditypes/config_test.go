// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package ditypes

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestParseConfigPath(t *testing.T) {
	expectedUUID, err := uuid.Parse("f0b49f3e-8364-448d-97e9-3e640c4a21e6")
	assert.NoError(t, err)

	configPath, err := ParseConfigPath("datadog/2/LIVE_DEBUGGING/logProbe_f0b49f3e-8364-448d-97e9-3e640c4a21e6/51fed9071414a7058c2ee96fc703f3e1fa51b5bffaab6155ce5c492303882b51")
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), configPath.OrgID)
	assert.Equal(t, "LIVE_DEBUGGING", configPath.Product)
	assert.Equal(t, "logProbe", configPath.ProbeType)
	assert.Equal(t, expectedUUID, configPath.ProbeUUID)
	assert.Equal(t, "51fed9071414a7058c2ee96fc703f3e1fa51b5bffaab6155ce5c492303882b51", configPath.Hash)
}

func TestParseConfigPathErrors(t *testing.T) {
	tcs := []string{
		"datadog/2/LIVE_DEBUGGING/logProbe_f0b49f3e-8364-448d-97e9-3e640c4a21e6",
		"datadog/2/NOT_SUPPORTED/logProbe_f0b49f3e-8364-448d-97e9-3e640c4a21e6/51fed9071414a7058c2ee96fc703f3e1fa51b5bffaab6155ce5c492303882b51",
		"datadog/2/LIVE_DEBUGGING/notSupported_f0b49f3e-8364-448d-97e9-3e640c4a21e6/51fed9071414a7058c2ee96fc703f3e1fa51b5bffaab6155ce5c492303882b51",
		"datadog/2/LIVE_DEBUGGING/logProbe_f0b49f3e-8364-448d-97e9-3e640c4a21e6/51fed9071414a7058c2ee96fc703f3e1fa51b5bffaab6155ce5c492303882b51/extra",
		"datadog/2/LIVE_DEBUGGING/logProbe_f0b49f3e-xxxx-448d-97e9-3e640c4a21e6/51fed9071414a7058c2ee96fc703f3e1fa51b5bffaab6155ce5c492303882b51",
	}
	for _, tc := range tcs {
		_, err := ParseConfigPath(tc)
		assert.Error(t, err)
	}
}
