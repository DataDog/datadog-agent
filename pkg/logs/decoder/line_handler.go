// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

// truncatedFlag is the flag that is added at the beginning
// or/and at the end of every trucated lines.
var truncatedFlag = []byte("...TRUNCATED...")

// escapedLineFeed is used to escape new line character
// for multiline message.
// New line character needs to be escaped because they are used
// as delimiter for transport.
var escapedLineFeed = []byte(`\n`)

// LineHandler handles raw lines to form structured lines
type LineHandler interface {
	Handle(input *Message)
	Start()
	Stop()
}
