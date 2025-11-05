// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

const nanoToMillis = 1000000

// StartMessageTranslator starts a goroutine that translates message.Message to message.StatefulMessage
// This is temporary scaffolding until the real State component is ready.
func StartMessageTranslator(inputChan chan *message.Message, outputChan chan *message.StatefulMessage) {
	go func() {
		defer close(outputChan)

		for msg := range inputChan {
			// Get timestamp - prefer message timestamp if available
			ts := time.Now().UTC()
			if !msg.ServerlessExtra.Timestamp.IsZero() {
				ts = msg.ServerlessExtra.Timestamp
			}

			// Create the Log message using stateful_encoding.proto definitions
			log := &statefulpb.Log{
				Timestamp: uint64(ts.UnixNano() / nanoToMillis),
				Content: &statefulpb.Log_Raw{
					Raw: toValidUtf8(msg.GetContent()),
				},
			}

			// Wrap the Log in a Datum
			datum := &statefulpb.Datum{
				Data: &statefulpb.Datum_Logs{
					Logs: log,
				},
			}

			// Create StatefulMessage with the Datum and metadata
			statefulMsg := &message.StatefulMessage{
				Datum:    datum,
				Metadata: &msg.MessageMetadata,
			}

			outputChan <- statefulMsg
		}
	}()
}

// toValidUtf8 ensures all characters are UTF-8
func toValidUtf8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}

	var str strings.Builder
	str.Grow(len(data))

	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		// in case of invalid utf-8, DecodeRune returns (utf8.RuneError, 1)
		// and since RuneError is the same as unicode.ReplacementChar
		// no need to handle the error explicitly
		str.WriteRune(r)
		data = data[size:]
	}
	return str.String()
}
