// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// truncatedFlag is the flag that is added at the beginning
// or/and at the end of every trucated lines.
var truncatedFlag = []byte("...TRUNCATED...")

// escapedLineFeed is used to escape new line character
// for multiline message.
// New line character needs to be escaped because they are used
// as delimiter for transport.
var escapedLineFeed = []byte(`\n`)

// LineHandler handles raw lines to form structured lines.
type LineHandler interface {
	// process handles a new line (message)
	process(*message.Message)

	// flushChan returns a channel which will deliver a message when `flush` should be called.
	flushChan() <-chan time.Time

	// flush flushes partially-processed data.  It should be called either when flushChan has
	// a message, or when the decoder is stopped.
	flush()
}
