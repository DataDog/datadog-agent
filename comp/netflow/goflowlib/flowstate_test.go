// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/netflow/config"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

func TestStartFlowRoutine_invalidType(t *testing.T) {
	logger := logmock.New(t)
	listenerErr := atomic.NewString("")
	listenerFlowCount := atomic.NewInt64(0)

	state, err := StartFlowRoutine("invalid", "my-hostname", 1234, 1, "my-ns", []config.Mapping{}, make(chan *common.Flow), logger, listenerErr, listenerFlowCount)

	assert.EqualError(t, err, "unknown flow type: invalid")
	assert.Nil(t, state)
}
