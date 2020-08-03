// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package api

import (
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/stretchr/testify/assert"
)

func TestMessageTypeToString(t *testing.T) {
	cases := map[model.MessageType]string{
		model.TypeCollectorPod:        "pod",
		model.TypeCollectorReplicaSet: "replica-set",
		model.TypeResCollector:        "23",
	}
	for input, expected := range cases {
		assert.Equal(t, messageTypeToString(input), expected)
	}
}
