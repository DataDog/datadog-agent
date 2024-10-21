// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/stretchr/testify/assert"
)

func TestDirection(t *testing.T) {
	assert.Equal(t, "ingress", Direction(uint32(0), common.TypeNetFlow9))
	assert.Equal(t, "egress", Direction(uint32(1), common.TypeNetFlow9))
	assert.Equal(t, "undefined", Direction(uint32(99), common.TypeNetFlow9))
	assert.Equal(t, "undefined", Direction(uint32(0), common.TypeNetFlow5))
	assert.Equal(t, "undefined", Direction(uint32(1), common.TypeNetFlow5))
	assert.Equal(t, "undefined", Direction(uint32(99), common.TypeNetFlow5))
}
