// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package processor

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pb"
)

// ProtoEncoder is a shared proto encoder.
var ProtoEncoder Encoder = &protoEncoder{}

// protoEncoder transforms a message into a protobuf byte array.
type protoEncoder struct{}

// Encode encodes a message into a protobuf byte array.
func (p *protoEncoder) Encode(msg *message.Message, redactedMsg []byte) ([]byte, error) {
	return (&pb.Log{
		Message:   toValidUtf8(redactedMsg),
		Status:    msg.GetStatus(),
		Timestamp: time.Now().UTC().UnixNano(),
		Hostname:  getHostname(),
		Service:   msg.Origin.Service(),
		Source:    msg.Origin.Source(),
		Tags:      msg.Origin.Tags(),
	}).Marshal()
}
