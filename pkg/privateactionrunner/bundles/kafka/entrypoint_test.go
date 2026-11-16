// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKafkaBundleActions(t *testing.T) {
	bundle := NewKafka()
	for _, action := range []string{"describeConsumerGroup", "resetConsumerGroupOffset", "testConnection"} {
		assert.NotNil(t, bundle.GetAction(action), action)
	}
	assert.Nil(t, bundle.GetAction("deleteConsumerGroup"))
}
