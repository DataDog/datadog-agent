// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// JSONServerlessEncoder is a shared json encoder sending a struct message field
// instead of a bytes message field. This encoder is used in the AWS Lambda
// serverless environment.
var JSONServerlessEncoder Encoder = &jsonServerlessEncoder{}

// jsonEncoder transforms a message into a JSON byte array.
type jsonServerlessEncoder struct{}

// JSON representation of a message.
type jsonServerlessPayload struct {
	Message   jsonServerlessMessage `json:"message"`
	Status    string                `json:"status"`
	Timestamp int64                 `json:"timestamp"`
	Hostname  string                `json:"hostname"`
	Service   string                `json:"service,omitempty"`
	Source    string                `json:"ddsource"`
	Tags      string                `json:"ddtags"`
}

type jsonServerlessMessage struct {
	Message string                `json:"message"`
	Lambda  *jsonServerlessLambda `json:"lambda,omitempty"`
}

type jsonServerlessLambda struct {
	ARN       string `json:"arn"`
	RequestID string `json:"request_id,omitempty"`
}

// Encode encodes a message into a JSON byte array.
func (j *jsonServerlessEncoder) Encode(msg *message.Message) error {
	if msg.State != message.StateRendered {
		return fmt.Errorf("message passed to encoder isn't rendered")
	}

	ts := time.Now().UTC()
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		ts = msg.ServerlessExtra.Timestamp
	}

	// add lambda metadata
	var lambdaPart *jsonServerlessLambda
	if l := msg.ServerlessExtra.Lambda; l != nil {
		lambdaPart = &jsonServerlessLambda{
			ARN:       l.ARN,
			RequestID: l.RequestID,
		}
	}

	encoded, err := json.Marshal(jsonServerlessPayload{
		Message: jsonServerlessMessage{
			Message: toValidUtf8(msg.GetContent()),
			Lambda:  lambdaPart,
		},
		Status:    msg.GetStatus(),
		Timestamp: ts.UnixNano() / nanoToMillis,
		Hostname:  msg.GetHostname(),
		Service:   msg.Origin.Service(),
		Source:    msg.Origin.Source(),
		Tags:      msg.Origin.TagsToString(),
	})

	if err != nil {
		return fmt.Errorf("can't encode the message: %v", err)
	}

	msg.SetEncoded(encoded)
	return nil
}
