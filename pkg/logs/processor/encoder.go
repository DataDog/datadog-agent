// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package processor

import (
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// Encoder turns a message into a raw byte array ready to be sent.
type Encoder interface {
	Encode(msg *message.Message, redactedMsg []byte) ([]byte, error)
}

// toValidUtf8 ensures all characters are UTF-8.
func toValidUtf8(msg []byte) string {
	if utf8.Valid(msg) {
		return string(msg)
	}
	str := make([]rune, 0, len(msg))
	for i := range msg {
		r, size := utf8.DecodeRune(msg[i:])
		if r == utf8.RuneError && size == 1 {
			str = append(str, unicode.ReplacementChar)
		} else {
			str = append(str, r)
		}
	}
	return string(str)
}

// getHostname returns the name of the host.
func getHostname() string {
	hostname, err := util.GetHostname()
	if err != nil {
		// this scenario is not likely to happen since
		// the agent can not start without a hostname
		hostname = "unknown"
	}
	return hostname
}
