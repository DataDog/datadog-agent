// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bytes"
	"fmt"
	"sync"
)

// lazyError defers building its message until Error is called, so the log
// functions can return an error without paying the formatting and scrubbing
// cost when the log level is disabled and the message is never used. The
// message is built at most once.
type lazyError struct {
	once  sync.Once
	msg   string
	build func() string
}

func (e *lazyError) Error() string {
	e.once.Do(func() { e.msg = e.build() })
	return e.msg
}

// lazyErrorv returns an error carrying the message lazily built from v.
func lazyErrorv(v ...interface{}) error {
	return &lazyError{build: func() string { return scrubMessage(fmt.Sprint(v...)) }}
}

// lazyErrorf returns an error carrying the message lazily built from format and params.
func lazyErrorf(format string, params ...interface{}) error {
	return &lazyError{build: func() string { return scrubMessage(fmt.Sprintf(format, params...)) }}
}

// lazyErrorc returns an error carrying the context message lazily built from message and context.
func lazyErrorc(message string, context ...interface{}) error {
	return &lazyError{build: func() string { return scrubMessage(formatContextEntry(message, context...)) }}
}

// scrubbedError returns an error carrying the lazily scrubbed msg.
func scrubbedError(msg string) error {
	return &lazyError{build: func() string { return scrubMessage(msg) }}
}

// formatContextEntry formats a message with its context as
// "message (key:value, key:value, ...)".
func formatContextEntry(message string, context ...interface{}) string {
	var fmtBuffer bytes.Buffer
	fmtBuffer.WriteString(message)
	if len(context) > 0 && len(context)%2 == 0 {
		fmtBuffer.WriteString(" (")
		for i := 0; i < len(context); i += 2 {
			fmtBuffer.WriteString("%s:%v")
			if i != len(context)-2 {
				fmtBuffer.WriteString(", ")
			}
		}
		fmtBuffer.WriteString(")")
	}

	return fmt.Sprintf(fmtBuffer.String(), context...)
}
