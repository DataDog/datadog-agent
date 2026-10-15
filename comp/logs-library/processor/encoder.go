// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processor provides log message processing functionality
package processor

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Encoder turns a message into a raw byte array ready to be sent.
type Encoder interface {
	Encode(msg *message.Message, hostname string) error
}

// Tap receives every rendered message before it is encoded for the primary
// destination, so a secondary destination with its own wire format can consume
// the shared processing output.
//
// Tap exists because Encoder cannot express flow control: Encode returns only
// an error, so an encoder-shaped hand-off has no way to say "wait, I am full"
// and is forced to shed. A Tap may block, which lets a secondary destination
// apply back-pressure like the primary one instead of losing logs.
//
// Implementations must not mutate msg; the primary destination's encoder
// encodes it in place afterwards.
type Tap interface {
	Tap(msg *message.Message)
}

type ValidUtf8Bytes []byte

func (msg ValidUtf8Bytes) MarshalText() (text []byte, err error) {
	if utf8.Valid(msg) {
		return msg, nil
	}

	var buf bytes.Buffer
	buf.Grow(len(msg))

	for len(msg) > 0 {
		r, size := utf8.DecodeRune(msg)
		// in case of invalid utf-8, DecodeRune returns (utf8.RuneError, 1)
		// and since RuneError is the same as unicode.ReplacementChar
		// no need to handle the error explicitly
		buf.WriteRune(r)
		msg = msg[size:]
	}
	return buf.Bytes(), nil
}

func (msg *ValidUtf8Bytes) UnmarshalText(text []byte) error {
	*msg = bytes.Clone(text)
	return nil
}

func (msg ValidUtf8Bytes) String() string {
	return string(msg)
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
