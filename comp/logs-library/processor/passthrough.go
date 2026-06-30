// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import "github.com/DataDog/datadog-agent/pkg/logs/message"

// PassthroughEncoder renders the message and stores the rendered bytes as the
// encoded content without wrapping them in a transport envelope (e.g. JSON).
// Use this when the consumer reads the content directly (e.g. the observer).
var PassthroughEncoder Encoder = &passthroughEncoder{}

type passthroughEncoder struct{}

func (p *passthroughEncoder) Encode(msg *message.Message, _ string) error {
	content, err := msg.RenderMessage()
	if err != nil {
		return err
	}
	msg.SetEncoded(content)
	return nil
}
