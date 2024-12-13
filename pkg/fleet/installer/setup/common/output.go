// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import "io"

type Output struct {
	tty io.Writer
}

func (o *Output) WriteString(s string) {
	o.tty.Write([]byte(s))
}
