// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"fmt"
	"time"

	"github.com/DataDog/agent-payload/v5/pb"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// ProtoEncoder is a shared proto encoder.
var ProtoEncoder Encoder = &protoEncoder{}

// protoEncoder transforms a message into a protobuf byte array.
type protoEncoder struct{}

// Encode encodes a message into a protobuf byte array.
func (p *protoEncoder) Encode(msg *message.Message) error {
	if msg.State != message.StateRendered {
		return fmt.Errorf("message passed to encoder isn't rendered")
	}

	log := &pb.Log{
		Message:   toValidUtf8(msg.GetContent()),
		Status:    msg.GetStatus(),
		Timestamp: time.Now().UTC().UnixNano(),
		Hostname:  msg.GetHostname(),
		Service:   msg.Origin.Service(),
		Source:    msg.Origin.Source(),
		Tags:      msg.Origin.Tags(),
	}
	encoded, err := log.Marshal()

	if err != nil {
		return fmt.Errorf("can't encode the message: %v", err)
	}

	msg.SetEncoded(encoded)
	return nil
}
