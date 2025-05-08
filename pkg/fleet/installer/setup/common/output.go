// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import "io"

// Output is a writer for the output. It will support some ANSI escape sequences to format the output.
type Output struct {
	tty io.Writer
}

// WriteString writes a string to the output.
func (o *Output) WriteString(s string) {
	if o.tty != nil {
		_, _ = o.tty.Write([]byte(s))
	}
}
