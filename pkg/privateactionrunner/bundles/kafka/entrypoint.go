// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package com_datadoghq_kafka implements the com.datadoghq.kafka bundle, which
// inspects and resets Kafka consumer group offsets.
package com_datadoghq_kafka

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Kafka struct {
	actions map[string]types.Action
}

func NewKafka() *Kafka {
	return &Kafka{
		actions: map[string]types.Action{
			"describeConsumerGroup":    NewDescribeConsumerGroupHandler(),
			"resetConsumerGroupOffset": NewResetConsumerGroupOffsetHandler(),
			"testConnection":           NewTestConnectionHandler(),
		},
	}
}

func (k *Kafka) GetAction(actionName string) types.Action {
	return k.actions[actionName]
}
