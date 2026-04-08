// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
)

// MockEncoder is a pass-through encoder for gRPC stateful streaming.
// It stores the hostname in the message metadata for downstream use.
// Actual encoding happens in MessageTranslator instead of the processor.
var MockEncoder processor.Encoder = &mockEncoder{}

type mockEncoder struct{}

// Encode stores the hostname in the message metadata for downstream use by MessageTranslator.
// The actual stateful encoding happens in MessageTranslator.processMessage.
func (g *mockEncoder) Encode(msg *message.Message, hostname string) error {
	// Store hostname in the message metadata so MessageTranslator can access it
	msg.MessageMetadata.Hostname = hostname
	return nil
}
