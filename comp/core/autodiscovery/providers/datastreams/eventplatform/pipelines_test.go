// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatform

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetECSFargateTaskARN(t *testing.T) {
	t.Run("returns empty when not on fargate", func(t *testing.T) {
		assert.Empty(t, getECSFargateTaskARN())
	})

	t.Run("returns empty when on fargate but metadata unavailable", func(t *testing.T) {
		t.Setenv("ECS_FARGATE", "true")
		assert.Empty(t, getECSFargateTaskARN())
	})
}
