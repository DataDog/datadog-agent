// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains types related to the eventplatform
package types

import "github.com/DataDog/datadog-agent/pkg/logs/message"

// An EventPlatformForwarder forwards Messages to a destination based on their event type
type EventPlatformForwarder interface {
	SendEventPlatformEvent(e *message.Message, eventType string) error
	SendEventPlatformEventBlocking(e *message.Message, eventType string) error
	Purge() map[string][]*message.Message
	Start()
	Stop()
}
