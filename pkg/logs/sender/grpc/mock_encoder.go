// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
)

// MockEncoder is a no-op encoder for gRPC stateful streaming.
// This is temporary scaffolding until the real State component is ready.
// Encoding happens in StartMessageTranslator instead of the processor.
var MockEncoder processor.Encoder = &mockEncoder{}

type mockEncoder struct{}

// Encode is a no-op implementation that satisfies the processor.Encoder interface
func (g *mockEncoder) Encode(_ *message.Message, _ string) error {
	return nil
}
