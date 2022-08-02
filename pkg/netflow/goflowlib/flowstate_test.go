// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
)

func TestStartFlowRoutine_invalidType(t *testing.T) {
	state, err := StartFlowRoutine("invalid", "my-hostname", 1234, 1, "my-ns", make(chan *common.Flow))
	assert.EqualError(t, err, "unknown flow type: invalid")
	assert.Nil(t, state)
}
