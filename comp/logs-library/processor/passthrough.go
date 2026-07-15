// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import "github.com/DataDog/datadog-agent/pkg/logs/message"

// PassthroughEncoder is an encoder that preserves the rendered log line in
// GetContent() unchanged. Use this when the consumer reads GetContent()
// directly (e.g. the observer) to avoid wrapping the content in a JSON
// transport envelope.
var PassthroughEncoder Encoder = &passthroughEncoder{}

type passthroughEncoder struct{}

func (p *passthroughEncoder) Encode(msg *message.Message, _ string) error {
	msg.SetEncoded(msg.GetContent())
	return nil
}
