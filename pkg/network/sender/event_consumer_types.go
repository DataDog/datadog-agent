// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import "github.com/DataDog/datadog-agent/pkg/eventmonitor"

// EventConsumerRegistry is the interface for an eventmonitor which allows adding handlers
type EventConsumerRegistry interface {
	AddEventConsumerHandler(consumer eventmonitor.EventConsumerHandler) error
}
