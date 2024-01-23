// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// ProtoEncoder is a shared proto encoder.
var ProtoEncoder Encoder = &protoEncoder{}

// protoEncoder transforms a message into a protobuf byte array.
type protoEncoder struct{}

// Encode encodes a message into a protobuf byte array.
func (p *protoEncoder) Encode(msg *message.Message) error {
	panic("not called")
}
