// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverlessinit

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

type eligibleProvider bool

func (p eligibleProvider) CanCollectMetadata() bool        { return bool(p) }
func (eligibleProvider) GetMetadata() CloudServiceMetadata { return CloudServiceMetadata{} }

func TestEligible(t *testing.T) {
	assert.False(t, Eligible(config.NewMock(t), eligibleProvider(true)), "default off")
	conf := config.NewMockWithOverrides(t, map[string]any{"serverless.inventory_enabled": true})
	assert.False(t, Eligible(conf, eligibleProvider(false)))
	assert.True(t, Eligible(conf, eligibleProvider(true)))
}
