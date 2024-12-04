// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_leaderStatusToRole(t *testing.T) {
	assert.Equal(t, "leader", leaderStateToRole(true))
	assert.Equal(t, "follower", leaderStateToRole(false))
}
