// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package state

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMergeRCConfigWithEmptyData(t *testing.T) {
	emptyUpdateStatus := func(_ string, _ ApplyStatus) {}

	content, err := MergeRCAgentConfig(emptyUpdateStatus, make(map[string]RawConfig))
	assert.NoError(t, err)
	assert.Equal(t, ConfigContent{}, content)
}
