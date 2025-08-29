// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processor provides log message processing functionality
package processor

import (
	"strings"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Encoder turns a message into a raw byte array ready to be sent.
type Encoder interface {
	Encode(msg *message.Message, hostname string) error
}

// toValidUtf8 ensures all characters are UTF-8.
func toValidUtf8(msg []byte) string {
	if utf8.Valid(msg) {
		return string(msg)
	}

	var str strings.Builder
	str.Grow(len(msg))

	for len(msg) > 0 {
		r, size := utf8.DecodeRune(msg)
		// in case of invalid utf-8, DecodeRune returns (utf8.RuneError, 1)
		// and since RuneError is the same as unicode.ReplacementChar
		// no need to handle the error explicitly
		str.WriteRune(r)
		msg = msg[size:]
	}
	return str.String()
}
